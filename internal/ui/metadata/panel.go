package metadata

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

// KeyValue represents a single metadata key-value pair.
type KeyValue struct {
	Key   string
	Value string
}

// MetadataPanel manages request and response metadata (headers).
// Request metadata is editable; response metadata is read-only.
type MetadataPanel struct {
	widget.BaseWidget

	// Request metadata (editable)
	requestKeys binding.StringList
	requestVals binding.StringList
	requestList *widget.List
	keyEntry    *widget.Entry
	valEntry    *widget.Entry
	addBtn      *widget.Button
	clearBtn    *widget.Button

	// Response metadata (read-only, shown after RPC)
	responseKeys binding.StringList
	responseVals binding.StringList
	responseList *widget.List
}

// NewMetadataPanel creates a new metadata panel.
func NewMetadataPanel() *MetadataPanel {
	p := &MetadataPanel{
		requestKeys:  binding.NewStringList(),
		requestVals:  binding.NewStringList(),
		responseKeys: binding.NewStringList(),
		responseVals: binding.NewStringList(),
	}

	// Request metadata list
	p.requestList = widget.NewList(
		func() int {
			return p.requestKeys.Length()
		},
		func() fyne.CanvasObject {
			// Template row: key label, equals, value label, delete button
			return container.NewHBox(
				widget.NewLabel(""),
				widget.NewLabel(" = "),
				widget.NewLabel(""),
				widget.NewButton("X", nil),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			hbox := obj.(*fyne.Container)
			keyLabel := hbox.Objects[0].(*widget.Label)
			valLabel := hbox.Objects[2].(*widget.Label)
			deleteBtn := hbox.Objects[3].(*widget.Button)

			// Get key and value from bindings
			key, _ := p.requestKeys.GetValue(id)
			val, _ := p.requestVals.GetValue(id)

			keyLabel.SetText(key)
			valLabel.SetText(val)

			// Wire delete button
			deleteBtn.OnTapped = func() {
				p.deleteRequestHeader(id)
			}
		},
	)

	// Response metadata list (read-only)
	p.responseList = widget.NewList(
		func() int {
			return p.responseKeys.Length()
		},
		func() fyne.CanvasObject {
			// Template row: key and value labels only (no delete button)
			return container.NewHBox(
				widget.NewLabel(""),
				widget.NewLabel(" = "),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			hbox := obj.(*fyne.Container)
			keyLabel := hbox.Objects[0].(*widget.Label)
			valLabel := hbox.Objects[2].(*widget.Label)

			// Get key and value from bindings
			key, _ := p.responseKeys.GetValue(id)
			val, _ := p.responseVals.GetValue(id)

			keyLabel.SetText(key)
			valLabel.SetText(val)
		},
	)

	// Entry fields for adding new request headers
	p.keyEntry = widget.NewEntry()
	p.keyEntry.SetPlaceHolder("Header name")

	p.valEntry = widget.NewEntry()
	p.valEntry.SetPlaceHolder("Header value")

	// Add and clear buttons
	p.addBtn = widget.NewButton("+ Add", func() {
		p.addRequestHeader()
	})

	p.clearBtn = widget.NewButton("Clear All", func() {
		p.ClearRequestMetadata()
	})

	p.ExtendBaseWidget(p)
	return p
}

// addRequestHeader adds a new request header.
func (p *MetadataPanel) addRequestHeader() {
	key := p.keyEntry.Text
	val := p.valEntry.Text

	if key == "" {
		return // Don't add empty keys
	}

	// Add to bindings
	_ = p.requestKeys.Append(key)
	_ = p.requestVals.Append(val)

	// Clear entry fields
	p.keyEntry.SetText("")
	p.valEntry.SetText("")

	p.requestList.Refresh()
}

// deleteRequestHeader removes a request header by index.
func (p *MetadataPanel) deleteRequestHeader(index int) {
	// Get current lists
	keys, _ := p.requestKeys.Get()
	vals, _ := p.requestVals.Get()

	if index < 0 || index >= len(keys) {
		return
	}

	// Remove element at index
	newKeys := append(keys[:index], keys[index+1:]...)
	newVals := append(vals[:index], vals[index+1:]...)

	// Update bindings
	_ = p.requestKeys.Set(newKeys)
	_ = p.requestVals.Set(newVals)

	p.requestList.Refresh()
}

// GetRequestMetadata returns the request metadata as a map for RPC.
func (p *MetadataPanel) GetRequestMetadata() map[string]string {
	metadata := make(map[string]string)

	length := p.requestKeys.Length()
	for i := 0; i < length; i++ {
		key, _ := p.requestKeys.GetValue(i)
		val, _ := p.requestVals.GetValue(i)
		metadata[key] = val
	}

	return metadata
}

// SetResponseMetadata displays response headers received from the server.
// This clears any previous response metadata and displays the new headers.
func (p *MetadataPanel) SetResponseMetadata(md map[string]string) {
	// Clear previous response metadata
	_ = p.responseKeys.Set([]string{})
	_ = p.responseVals.Set([]string{})

	// Add new metadata
	for key, val := range md {
		_ = p.responseKeys.Append(key)
		_ = p.responseVals.Append(val)
	}

	p.responseList.Refresh()
}

// ClearRequestMetadata clears all request headers.
func (p *MetadataPanel) ClearRequestMetadata() {
	_ = p.requestKeys.Set([]string{})
	_ = p.requestVals.Set([]string{})
	p.requestList.Refresh()
}

// ClearResponseMetadata clears all response headers.
func (p *MetadataPanel) ClearResponseMetadata() {
	_ = p.responseKeys.Set([]string{})
	_ = p.responseVals.Set([]string{})
	p.responseList.Refresh()
}

// Clear resets both request and response metadata.
func (p *MetadataPanel) Clear() {
	p.ClearRequestMetadata()
	p.ClearResponseMetadata()
}

// CreateRenderer implements fyne.Widget.
func (p *MetadataPanel) CreateRenderer() fyne.WidgetRenderer {
	// Request headers section
	requestLabel := widget.NewLabel("Request Headers:")
	requestLabel.TextStyle = fyne.TextStyle{Bold: true}

	requestEntry := container.NewBorder(
		nil, nil, nil,
		container.NewHBox(p.addBtn, p.clearBtn),
		container.NewGridWithColumns(2, p.keyEntry, p.valEntry),
	)

	requestSection := container.NewBorder(
		container.NewVBox(requestLabel, widget.NewSeparator()),
		requestEntry,
		nil, nil,
		p.requestList,
	)

	// Response headers section
	responseLabel := widget.NewLabel("Response Headers:")
	responseLabel.TextStyle = fyne.TextStyle{Bold: true}

	responseSection := container.NewBorder(
		container.NewVBox(responseLabel, widget.NewSeparator()),
		nil, nil, nil,
		p.responseList,
	)

	// Main layout: request on top, response on bottom
	content := container.NewVBox(
		requestSection,
		widget.NewSeparator(),
		responseSection,
	)

	return widget.NewSimpleRenderer(content)
}
