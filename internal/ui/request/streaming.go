package request

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// StreamingInputWidget provides UI for client streaming RPCs.
// It allows sending multiple messages and then finishing the stream to get a response.
type StreamingInputWidget struct {
	widget.BaseWidget

	messageEntry *widget.Entry       // Current message to send (multiline JSON editor)
	sentList     *widget.List        // List of sent messages
	sentMessages binding.StringList  // Binding for sent messages

	sendBtn   *widget.Button // Send current message
	finishBtn *widget.Button // Close stream and get response

	onSend   func(json string) // Callback when Send is clicked
	onFinish func()             // Callback when Finish is clicked
}

// NewStreamingInputWidget creates a new streaming input widget.
func NewStreamingInputWidget() *StreamingInputWidget {
	w := &StreamingInputWidget{
		sentMessages: binding.NewStringList(),
	}

	// Message entry - multiline JSON editor
	w.messageEntry = widget.NewMultiLineEntry()
	w.messageEntry.SetPlaceHolder(`{"field": "value"}`)
	w.messageEntry.Wrapping = fyne.TextWrapWord

	// List of sent messages
	w.sentList = widget.NewList(
		func() int {
			return w.sentMessages.Length()
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			msg, _ := w.sentMessages.GetValue(id)
			label.SetText(msg)
		},
	)

	// Buttons
	w.sendBtn = widget.NewButton("Send Message", func() {
		w.handleSend()
	})

	w.finishBtn = widget.NewButton("Finish & Get Response", func() {
		w.handleFinish()
	})
	w.finishBtn.Importance = widget.HighImportance

	w.ExtendBaseWidget(w)
	return w
}

// SetOnSend sets the callback for when a message is sent.
func (w *StreamingInputWidget) SetOnSend(fn func(json string)) {
	w.onSend = fn
}

// SetOnFinish sets the callback for when the stream is finished.
func (w *StreamingInputWidget) SetOnFinish(fn func()) {
	w.onFinish = fn
}

// handleSend sends the current message and adds it to the sent list.
func (w *StreamingInputWidget) handleSend() {
	if w.onSend == nil {
		return
	}

	msg := w.messageEntry.Text
	if msg == "" {
		return // Don't send empty messages
	}

	// Call the callback
	w.onSend(msg)

	// Add to sent messages list
	_ = w.sentMessages.Append(msg)

	// Clear the entry for next message
	w.messageEntry.SetText("")

	// Refresh the list
	w.sentList.Refresh()
}

// handleFinish closes the stream and requests the final response.
func (w *StreamingInputWidget) handleFinish() {
	if w.onFinish == nil {
		return
	}

	w.onFinish()
}

// AddSent adds a sent message to the list (for programmatic use).
func (w *StreamingInputWidget) AddSent(json string) {
	_ = w.sentMessages.Append(json)
	w.sentList.Refresh()
}

// Clear resets the widget for a new stream.
func (w *StreamingInputWidget) Clear() {
	w.messageEntry.SetText("")
	w.sentMessages = binding.NewStringList()
	w.sentList.Refresh()
}

// GetCurrentMessage returns the current message text.
func (w *StreamingInputWidget) GetCurrentMessage() string {
	return w.messageEntry.Text
}

// SetCurrentMessage sets the current message text.
func (w *StreamingInputWidget) SetCurrentMessage(text string) {
	w.messageEntry.SetText(text)
}

// CreateRenderer implements fyne.Widget.
func (w *StreamingInputWidget) CreateRenderer() fyne.WidgetRenderer {
	// Sent messages section
	sentCountLabel := widget.NewLabel("Sent messages:")
	sentCountLabel.TextStyle = fyne.TextStyle{Bold: true}

	sentSection := container.NewBorder(
		sentCountLabel,
		nil, nil, nil,
		w.sentList,
	)

	// Next message section
	nextLabel := widget.NewLabel("Next message:")
	nextLabel.TextStyle = fyne.TextStyle{Bold: true}

	messageSection := container.NewBorder(
		nextLabel,
		nil, nil, nil,
		w.messageEntry,
	)

	// Buttons at bottom
	buttonBox := container.NewHBox(
		w.sendBtn,
		layout.NewSpacer(),
		w.finishBtn,
	)

	// Full layout: sent messages on top, next message in middle, buttons at bottom
	content := container.NewBorder(
		nil,       // top
		buttonBox, // bottom (buttons)
		nil, nil,  // left, right
		container.NewVSplit(
			sentSection,    // top half (sent messages)
			messageSection, // bottom half (next message)
		),
	)

	return widget.NewSimpleRenderer(content)
}

// MinSize implements fyne.Widget.
func (w *StreamingInputWidget) MinSize() fyne.Size {
	return fyne.NewSize(400, 400)
}
