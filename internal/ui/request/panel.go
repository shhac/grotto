package request

import (
	"bytes"
	"encoding/json"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/model"
	"github.com/shhac/grotto/internal/ui/components"
	"github.com/shhac/grotto/internal/ui/form"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// RequestPanel handles request input.
//
// SYNC ARCHITECTURE:
// Mode switching (Text <-> Form) is handled by ModeSynchronizer.
// See mode_sync.go for detailed documentation on how sync works.
//
// Key points:
//   - ModeSynchronizer owns the syncing flag and all sync logic
//   - SetOnModeChange calls synchronizer.SwitchMode()
//   - state.Mode listener checks synchronizer.IsSyncing() before acting
//   - This prevents infinite loops that cause UI freezes
type RequestPanel struct {
	widget.BaseWidget

	state       *model.RequestState
	methodLabel *widget.Label

	// Text mode
	textEditor *widget.Entry // Multiline JSON editor

	// Form mode
	formBuilder     *form.FormBuilder              // Form generator
	formPlaceholder *widget.Label                  // Shown when no method selected
	formContainer   *fyne.Container                // Container for form or placeholder
	currentDesc     protoreflect.MessageDescriptor // Current message descriptor

	// Mode synchronization (prevents freeze bugs)
	synchronizer *ModeSynchronizer

	// Mode tabs
	modeTabs *components.ModeTabs // Text/Form mode toggle

	// Streaming mode
	streamingInput *StreamingInputWidget // Client streaming input widget
	isStreaming    bool                  // Whether current method is client streaming

	// Metadata
	metadataKeys binding.StringList // Keys for metadata
	metadataVals binding.StringList // Values for metadata
	metadataList *widget.List       // Key-value metadata entries
	keyEntry     *widget.Entry      // New key entry
	valEntry     *widget.Entry      // New value entry
	sendBtn      *widget.Button

	// Top-level tabs (Request Body | Request Metadata)
	topLevelTabs    *container.AppTabs
	bodyTab         *container.TabItem
	metadataTab     *container.TabItem
	bodyTabContent  *fyne.Container
	metadataContent *fyne.Container

	// Full layout container returned by CreateRenderer
	content *fyne.Container

	logger *slog.Logger

	onSend       func(json string, metadata map[string]string)
	onStreamSend func(json string, metadata map[string]string) // Send one message in stream
	onStreamEnd  func(metadata map[string]string)              // Finish stream and get response
}

// NewRequestPanel creates a new request panel
func NewRequestPanel(state *model.RequestState, logger *slog.Logger) *RequestPanel {
	p := &RequestPanel{
		state:        state,
		metadataKeys: binding.NewStringList(),
		metadataVals: binding.NewStringList(),
		logger:       logger,
	}

	// Create mode synchronizer (handles Text <-> Form sync)
	p.synchronizer = NewModeSynchronizer(state.Mode, state.TextData, logger)

	// Method label shows which method is selected
	p.methodLabel = widget.NewLabel("No method selected")
	p.methodLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Multiline JSON editor bound to state.TextData
	p.textEditor = widget.NewMultiLineEntry()
	p.textEditor.SetPlaceHolder(`{"field": "value"}`)
	p.textEditor.Wrapping = fyne.TextWrapWord
	p.textEditor.Bind(state.TextData)

	// Form mode placeholder
	p.formPlaceholder = widget.NewLabel("Select a method to see the form")
	p.formPlaceholder.Alignment = fyne.TextAlignCenter
	p.formContainer = container.NewMax(container.NewCenter(p.formPlaceholder))

	// Create mode tabs with text editor and form container
	p.modeTabs = components.NewModeTabs(
		container.NewMax(p.textEditor),
		p.formContainer,
	)

	// Listen for tab changes - delegate to synchronizer
	p.modeTabs.SetOnModeChange(func(mode string) {
		p.synchronizer.SwitchMode(mode)
	})

	// Listen for state.Mode changes (programmatic changes from outside)
	state.Mode.AddListener(binding.NewDataListener(func() {
		// Skip if synchronizer is handling a mode change
		if p.synchronizer.IsSyncing() {
			return
		}
		mode, _ := state.Mode.Get()
		if p.modeTabs.GetMode() != mode {
			p.modeTabs.SetMode(mode)
		}
	}))

	// Metadata list showing key-value pairs
	p.metadataList = widget.NewList(
		func() int {
			return p.metadataKeys.Length()
		},
		func() fyne.CanvasObject {
			// Template row: two labels for key and value
			return container.NewHBox(
				widget.NewLabel(""),
				widget.NewLabel("="),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			keyLabel := box.Objects[0].(*widget.Label)
			valLabel := box.Objects[2].(*widget.Label)

			// Get key and value from bindings
			key, _ := p.metadataKeys.GetValue(id)
			val, _ := p.metadataVals.GetValue(id)

			keyLabel.SetText(key)
			valLabel.SetText(val)
		},
	)

	// New metadata entry fields
	p.keyEntry = widget.NewEntry()
	p.keyEntry.SetPlaceHolder("Header name")

	p.valEntry = widget.NewEntry()
	p.valEntry.SetPlaceHolder("Header value")

	// Send button (disabled until a method is selected)
	p.sendBtn = widget.NewButton("Send", func() {
		p.handleSend()
	})
	p.sendBtn.Disable()

	// Streaming input widget
	p.streamingInput = NewStreamingInputWidget()
	p.streamingInput.SetOnSend(func(json string) {
		p.handleStreamSend(json)
	})
	p.streamingInput.SetOnFinish(func() {
		p.handleStreamFinish()
	})

	p.initializeComponents()
	p.ExtendBaseWidget(p)
	return p
}

// initializeComponents creates layout containers once, following the same
// pattern as ResponsePanel and BidiStreamPanel. This avoids recreating
// widgets inside CreateRenderer, which Fyne may call more than once.
func (p *RequestPanel) initializeComponents() {
	// Metadata section UI
	addMetadataBtn := widget.NewButton("+ Add Header", func() {
		p.addMetadata()
	})

	metadataEntry := container.NewBorder(
		nil, nil,
		nil, addMetadataBtn,
		container.NewGridWithColumns(2,
			p.keyEntry,
			p.valEntry,
		),
	)

	p.metadataContent = container.NewBorder(
		nil,
		metadataEntry,
		nil, nil,
		p.metadataList,
	)

	// Body tab content: swaps between modeTabs (normal) and streamingInput
	p.bodyTabContent = container.NewMax(p.modeTabs)

	// Single set of top-level tabs — no more shared TabItem across two AppTabs
	p.bodyTab = container.NewTabItem("Request Body", p.bodyTabContent)
	p.metadataTab = container.NewTabItem("Request Metadata", p.metadataContent)
	p.topLevelTabs = container.NewAppTabs(p.bodyTab, p.metadataTab)

	// Header row: method label on left, send button on right
	headerRow := container.NewBorder(nil, nil, nil, p.sendBtn, p.methodLabel)

	// Full layout
	p.content = container.NewBorder(
		container.NewVBox(
			headerRow,
			widget.NewSeparator(),
		),
		nil,
		nil, nil,
		p.topLevelTabs,
	)
}

// SetSendEnabled enables or disables the Send button
func (p *RequestPanel) SetSendEnabled(enabled bool) {
	if enabled {
		p.sendBtn.Enable()
	} else {
		p.sendBtn.Disable()
	}
}

// SetOnSend sets the callback for when Send is clicked (unary/server streaming)
func (p *RequestPanel) SetOnSend(fn func(json string, metadata map[string]string)) {
	p.onSend = fn
}

// SetOnStreamSend sets the callback for sending a message in client streaming
func (p *RequestPanel) SetOnStreamSend(fn func(json string, metadata map[string]string)) {
	p.onStreamSend = fn
}

// SetOnStreamEnd sets the callback for finishing a client stream
func (p *RequestPanel) SetOnStreamEnd(fn func(metadata map[string]string)) {
	p.onStreamEnd = fn
}

// StreamingInput returns the client streaming input widget.
func (p *RequestPanel) StreamingInput() *StreamingInputWidget {
	return p.streamingInput
}

// SetClientStreaming switches the panel to/from client streaming mode
func (p *RequestPanel) SetClientStreaming(streaming bool) {
	p.isStreaming = streaming
	if streaming {
		p.streamingInput.Clear()
		p.bodyTabContent.Objects = []fyne.CanvasObject{p.streamingInput}
		p.sendBtn.Hide()
	} else {
		p.bodyTabContent.Objects = []fyne.CanvasObject{p.modeTabs}
		p.sendBtn.Show()
	}
	p.bodyTabContent.Refresh()
}

// SetMethod updates the panel for a selected method
func (p *RequestPanel) SetMethod(methodName string, inputDesc protoreflect.MessageDescriptor) {
	if methodName == "" {
		p.methodLabel.SetText("No method selected")
		p.currentDesc = nil
		if p.formBuilder != nil {
			p.formBuilder.Destroy()
		}
		p.formBuilder = nil
		p.synchronizer.SetFormBuilder(nil)
		p.formContainer.Objects = []fyne.CanvasObject{container.NewCenter(p.formPlaceholder)}
		p.formContainer.Refresh()
	} else {
		p.methodLabel.SetText("Method: " + methodName)
		p.currentDesc = inputDesc

		// Build form for this method
		if inputDesc != nil {
			if p.formBuilder != nil {
				p.formBuilder.Destroy()
			}
			p.formBuilder = form.NewFormBuilder(inputDesc)
			p.synchronizer.SetFormBuilder(p.formBuilder)
			formUI := p.formBuilder.Build()
			p.formContainer.Objects = []fyne.CanvasObject{formUI}
			p.formContainer.Refresh()

			// Clear text data when switching methods - old JSON won't match new schema
			// This prevents crashes from trying to sync incompatible data
			_ = p.state.TextData.Set("")
		}
	}
	p.Refresh()
}

// addMetadata adds a new metadata header
func (p *RequestPanel) addMetadata() {
	key := p.keyEntry.Text
	val := p.valEntry.Text

	if key == "" {
		return // Don't add empty keys
	}

	// Add to bindings
	_ = p.metadataKeys.Append(key)
	_ = p.metadataVals.Append(val)

	// Clear entry fields
	p.keyEntry.SetText("")
	p.valEntry.SetText("")

	p.metadataList.Refresh()
}

// handleSend collects data and invokes the onSend callback (unary/server streaming)
func (p *RequestPanel) handleSend() {
	if p.onSend == nil {
		return
	}

	// If in form mode, sync form to text first
	currentMode, _ := p.state.Mode.Get()
	if currentMode == "form" && p.formBuilder != nil {
		p.synchronizer.SyncFormToTextNow()
	}

	// Get JSON text from state
	jsonText, _ := p.state.TextData.Get()

	// Pretty-print JSON
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(jsonText), "", "  "); err == nil {
		jsonText = buf.String()
	}

	// Build metadata map
	metadata := p.getMetadata()

	p.onSend(jsonText, metadata)
}

// handleStreamSend sends a single message in a client stream
func (p *RequestPanel) handleStreamSend(jsonText string) {
	if p.onStreamSend == nil {
		return
	}

	// Pretty-print JSON
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(jsonText), "", "  "); err == nil {
		jsonText = buf.String()
	}

	// Build metadata map
	metadata := p.getMetadata()

	p.onStreamSend(jsonText, metadata)
}

// handleStreamFinish finishes the client stream and requests the response
func (p *RequestPanel) handleStreamFinish() {
	if p.onStreamEnd == nil {
		return
	}

	// Build metadata map
	metadata := p.getMetadata()

	p.onStreamEnd(metadata)
}

// getMetadata builds the metadata map from the UI
func (p *RequestPanel) getMetadata() map[string]string {
	metadata := make(map[string]string)
	length := p.metadataKeys.Length()
	for i := 0; i < length; i++ {
		key, _ := p.metadataKeys.GetValue(i)
		val, _ := p.metadataVals.GetValue(i)
		metadata[key] = val
	}
	return metadata
}

// SetMetadata replaces the metadata entries displayed in the UI.
func (p *RequestPanel) SetMetadata(metadata map[string]string) {
	keys := make([]string, 0, len(metadata))
	vals := make([]string, 0, len(metadata))
	for k, v := range metadata {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	_ = p.metadataKeys.Set(keys)
	_ = p.metadataVals.Set(vals)
	p.metadataList.Refresh()
}

// SyncTextToForm populates the form from current TextData (for history load)
func (p *RequestPanel) SyncTextToForm() {
	p.synchronizer.SyncTextToFormNow()
}

// TriggerSend programmatically triggers the send action (for keyboard shortcut)
func (p *RequestPanel) TriggerSend() {
	p.handleSend()
}

// SwitchToTextMode switches to text mode (for keyboard shortcut)
func (p *RequestPanel) SwitchToTextMode() {
	p.modeTabs.SetMode("text")
}

// SwitchToFormMode switches to form mode (for keyboard shortcut)
func (p *RequestPanel) SwitchToFormMode() {
	p.modeTabs.SetMode("form")
}

// FocusEditor moves keyboard focus to the text editor widget.
func (p *RequestPanel) FocusEditor() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(p.textEditor); c != nil {
		c.Focus(p.textEditor)
	}
}

// CreateRenderer returns the widget renderer.
func (p *RequestPanel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}
