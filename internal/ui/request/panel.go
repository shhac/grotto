package request

import (
	"encoding/json"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/model"
	"github.com/shhac/grotto/internal/ui/components"
	"github.com/shhac/grotto/internal/ui/form"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// RequestPanel handles request input.
//
// SYNC ARCHITECTURE (to prevent regressions):
// The Text/Form mode sync uses a single 'syncing' flag to prevent infinite loops.
// The flow is:
//   1. User clicks tab OR keyboard shortcut triggers mode change
//   2. SetOnModeChange callback fires, sets syncing=true
//   3. state.Mode.Set() updates the binding
//   4. state.Mode listener fires but returns early (syncing=true)
//   5. syncModeData() runs to sync form↔text data
//   6. syncing=false (defer)
//
// IMPORTANT: syncModeData() must NOT have its own syncing guard - it runs
// while syncing=true and that's intentional.
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
	syncing         bool                           // Flag to prevent sync loops

	// Mode tabs
	modeTabs *components.ModeTabs // Text/Form mode toggle

	// Streaming mode
	streamingInput *StreamingInputWidget // Client streaming input widget
	isStreaming    bool                  // Whether current method is client streaming
	mainContainer  *fyne.Container       // Container that switches between normal/streaming

	// Metadata
	metadataKeys binding.StringList // Keys for metadata
	metadataVals binding.StringList // Values for metadata
	metadataList *widget.List       // Key-value metadata entries
	keyEntry     *widget.Entry      // New key entry
	valEntry     *widget.Entry      // New value entry
	sendBtn      *widget.Button

	// Top-level tabs (Request Body | Request Metadata)
	topLevelTabs     *container.AppTabs
	bodyTab          *container.TabItem
	metadataTab      *container.TabItem
	bodyTabContent   *fyne.Container
	metadataContent  *fyne.Container

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

	// Listen for tab changes and sync to state
	p.modeTabs.SetOnModeChange(func(mode string) {
		// Prevent sync loops when user clicks tab
		if p.syncing {
			return
		}
		p.syncing = true
		defer func() { p.syncing = false }()

		_ = p.state.Mode.Set(mode)
		p.syncModeData(mode)
	})

	// Listen for state.Mode changes (programmatic changes)
	state.Mode.AddListener(binding.NewDataListener(func() {
		// Prevent sync loops
		if p.syncing {
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

	// Send button
	p.sendBtn = widget.NewButton("Send", func() {
		p.handleSend()
	})

	// Streaming input widget
	p.streamingInput = NewStreamingInputWidget()
	p.streamingInput.SetOnSend(func(json string) {
		p.handleStreamSend(json)
	})
	p.streamingInput.SetOnFinish(func() {
		p.handleStreamFinish()
	})

	p.ExtendBaseWidget(p)
	return p
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

// SetClientStreaming switches the panel to/from client streaming mode
func (p *RequestPanel) SetClientStreaming(streaming bool) {
	p.isStreaming = streaming
	if streaming {
		// Clear the streaming widget for new stream
		p.streamingInput.Clear()
	}
	p.Refresh()
}

// SetMethod updates the panel for a selected method
func (p *RequestPanel) SetMethod(methodName string, inputDesc protoreflect.MessageDescriptor) {
	if methodName == "" {
		p.methodLabel.SetText("No method selected")
		p.currentDesc = nil
		p.formBuilder = nil
		p.formContainer.Objects = []fyne.CanvasObject{container.NewCenter(p.formPlaceholder)}
		p.formContainer.Refresh()
	} else {
		p.methodLabel.SetText("Method: " + methodName)
		p.currentDesc = inputDesc

		// Build form for this method
		if inputDesc != nil {
			p.formBuilder = form.NewFormBuilder(inputDesc)
			formUI := p.formBuilder.Build()
			p.formContainer.Objects = []fyne.CanvasObject{formUI}
			p.formContainer.Refresh()

			// If in form mode, sync existing text to form
			currentMode, _ := p.state.Mode.Get()
			if currentMode == "form" {
				p.syncTextToForm()
			}
		}
	}
	p.Refresh()
}

// syncModeData synchronizes data when switching between modes.
// NOTE: This function must NOT have its own syncing guard - the caller
// (SetOnModeChange callback) already sets syncing=true before calling this.
// Adding a guard here would cause the function to do nothing.
func (p *RequestPanel) syncModeData(mode string) {
	if mode == "form" {
		// Switching to form mode: parse text JSON and populate form
		p.syncTextToForm()
	} else if mode == "text" {
		// Switching to text mode: convert form to JSON
		p.syncFormToText()
	}
}

// syncTextToForm parses the text editor JSON and populates the form
func (p *RequestPanel) syncTextToForm() {
	if p.formBuilder == nil {
		return
	}

	textData, _ := p.state.TextData.Get()
	if textData == "" {
		return
	}

	// Try to parse and populate form
	if err := p.formBuilder.FromJSON(textData); err != nil {
		p.logger.Warn("failed to populate form from JSON", slog.Any("error", err))
	}
}

// syncFormToText converts form values to JSON and updates text editor
func (p *RequestPanel) syncFormToText() {
	if p.formBuilder == nil {
		return
	}

	// Get JSON from form
	jsonStr, err := p.formBuilder.ToJSON()
	if err != nil {
		p.logger.Warn("failed to convert form to JSON", slog.Any("error", err))
		return
	}

	// Update text data binding
	_ = p.state.TextData.Set(jsonStr)
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
		p.syncFormToText()
	}

	// Get JSON text from state
	jsonText, _ := p.state.TextData.Get()

	// Pretty-print JSON
	var prettyJSON interface{}
	if err := json.Unmarshal([]byte(jsonText), &prettyJSON); err == nil {
		prettyBytes, _ := json.MarshalIndent(prettyJSON, "", "  ")
		jsonText = string(prettyBytes)
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
	var prettyJSON interface{}
	if err := json.Unmarshal([]byte(jsonText), &prettyJSON); err == nil {
		prettyBytes, _ := json.MarshalIndent(prettyJSON, "", "  ")
		jsonText = string(prettyBytes)
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

// CreateRenderer returns the widget renderer
func (p *RequestPanel) CreateRenderer() fyne.WidgetRenderer {
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

	// Request body tab content (contains the existing Text/Form mode tabs)
	p.bodyTabContent = container.NewMax(p.modeTabs)

	// Create top-level tabs
	p.bodyTab = container.NewTabItem("Request Body", p.bodyTabContent)
	p.metadataTab = container.NewTabItem("Request Metadata", p.metadataContent)
	p.topLevelTabs = container.NewAppTabs(p.bodyTab, p.metadataTab)

	// Send button aligned to right (for unary/server streaming)
	sendBox := container.NewHBox(
		layout.NewSpacer(),
		p.sendBtn,
	)

	// Normal layout (unary/server streaming) - now uses top-level tabs
	normalLayout := p.topLevelTabs

	// Streaming layout (client streaming) - also uses top-level tabs
	// We'll switch the body tab content to show streaming input
	streamingBodyContent := container.NewMax(p.streamingInput)
	streamingBodyTab := container.NewTabItem("Request Body", streamingBodyContent)
	streamingTabs := container.NewAppTabs(streamingBodyTab, p.metadataTab)

	// Main container that switches between normal and streaming
	p.mainContainer = container.NewMax()
	if p.isStreaming {
		p.mainContainer.Objects = []fyne.CanvasObject{streamingTabs}
	} else {
		p.mainContainer.Objects = []fyne.CanvasObject{normalLayout}
	}

	// Full layout
	content := container.NewBorder(
		container.NewVBox(
			p.methodLabel,
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			sendBox,
		),
		nil, nil,
		p.mainContainer,
	)

	// Update visibility of sendBox based on streaming mode
	if p.isStreaming {
		sendBox.Hide()
	} else {
		sendBox.Show()
	}

	return widget.NewSimpleRenderer(content)
}
