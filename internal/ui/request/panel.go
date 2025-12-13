package request

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/model"
)

// RequestPanel handles request input
type RequestPanel struct {
	widget.BaseWidget

	state        *model.RequestState
	methodLabel  *widget.Label
	textEditor   *widget.Entry           // Multiline JSON editor
	metadataKeys binding.StringList      // Keys for metadata
	metadataVals binding.StringList      // Values for metadata
	metadataList *widget.List            // Key-value metadata entries
	keyEntry     *widget.Entry           // New key entry
	valEntry     *widget.Entry           // New value entry
	sendBtn      *widget.Button

	onSend func(json string, metadata map[string]string)
}

// NewRequestPanel creates a new request panel
func NewRequestPanel(state *model.RequestState) *RequestPanel {
	p := &RequestPanel{
		state:        state,
		metadataKeys: binding.NewStringList(),
		metadataVals: binding.NewStringList(),
	}

	// Method label shows which method is selected
	p.methodLabel = widget.NewLabel("No method selected")
	p.methodLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Multiline JSON editor bound to state.TextData
	p.textEditor = widget.NewMultiLineEntry()
	p.textEditor.SetPlaceHolder(`{"field": "value"}`)
	p.textEditor.Wrapping = fyne.TextWrapWord
	p.textEditor.Bind(state.TextData)

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

	p.ExtendBaseWidget(p)
	return p
}

// SetOnSend sets the callback for when Send is clicked
func (p *RequestPanel) SetOnSend(fn func(json string, metadata map[string]string)) {
	p.onSend = fn
}

// SetMethod updates the panel for a selected method
func (p *RequestPanel) SetMethod(methodName string, inputType string) {
	if methodName == "" {
		p.methodLabel.SetText("No method selected")
	} else {
		p.methodLabel.SetText("Method: " + methodName)
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

// handleSend collects data and invokes the onSend callback
func (p *RequestPanel) handleSend() {
	if p.onSend == nil {
		return
	}

	// Get JSON text from state
	jsonText, _ := p.state.TextData.Get()

	// Build metadata map
	metadata := make(map[string]string)
	length := p.metadataKeys.Length()
	for i := 0; i < length; i++ {
		key, _ := p.metadataKeys.GetValue(i)
		val, _ := p.metadataVals.GetValue(i)
		metadata[key] = val
	}

	p.onSend(jsonText, metadata)
}

// CreateRenderer returns the widget renderer
func (p *RequestPanel) CreateRenderer() fyne.WidgetRenderer {
	// Request body section
	requestLabel := widget.NewLabel("Request Body (JSON):")
	requestBox := container.NewBorder(
		requestLabel,
		nil, nil, nil,
		container.NewMax(p.textEditor),
	)

	// Metadata section
	metadataLabel := widget.NewLabel("Metadata:")
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

	metadataBox := container.NewBorder(
		metadataLabel,
		metadataEntry,
		nil, nil,
		p.metadataList,
	)

	// Send button aligned to right
	sendBox := container.NewHBox(
		layout.NewSpacer(),
		p.sendBtn,
	)

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
		container.NewVBox(
			requestBox,
			widget.NewSeparator(),
			metadataBox,
		),
	)

	return widget.NewSimpleRenderer(content)
}
