package request

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

const (
	maxStreamMessages = 1000
	evictionBatch     = 200
)

// StreamingInputWidget provides UI for client streaming RPCs.
// It allows sending multiple messages and then finishing the stream to get a response.
type StreamingInputWidget struct {
	widget.BaseWidget

	messageEntry *widget.Entry      // Current message to send (multiline JSON editor)
	sentList     *widget.List       // List of sent messages
	sentMessages binding.StringList // Binding for sent messages

	sendBtn   *widget.Button // Send current message
	finishBtn *widget.Button // Close stream and get response

	statusLabel *widget.Label // Status display
	totalSent   int           // Total sent including evicted

	onSend   func(json string) // Callback when Send is clicked
	onFinish func()            // Callback when Finish is clicked
}

// NewStreamingInputWidget creates a new streaming input widget.
func NewStreamingInputWidget() *StreamingInputWidget {
	w := &StreamingInputWidget{
		sentMessages: binding.NewStringList(),
		statusLabel:  widget.NewLabel("Ready"),
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
			entry := widget.NewMultiLineEntry()
			entry.Wrapping = fyne.TextWrapWord
			entry.Disable()
			return entry
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			msg, _ := w.sentMessages.GetValue(id)
			entry.SetText(msg)
		},
	)

	// Buttons
	w.sendBtn = widget.NewButton("Send Message", func() {
		w.handleSend()
	})

	w.finishBtn = widget.NewButton("Close Stream", func() {
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
	w.totalSent++

	// Evict oldest if over cap
	if count := w.sentMessages.Length(); count > maxStreamMessages {
		all, err := w.sentMessages.Get()
		if err == nil && len(all) > maxStreamMessages {
			_ = w.sentMessages.Set(all[evictionBatch:])
		}
	}

	// Clear the entry for next message
	w.messageEntry.SetText("")

	// Refresh the list
	w.sentList.Refresh()
	w.updateStatus()
}

// handleFinish closes the stream and requests the final response.
func (w *StreamingInputWidget) handleFinish() {
	if w.onFinish == nil {
		return
	}

	w.onFinish()
	w.sendBtn.Disable()
	w.finishBtn.Disable()
	w.messageEntry.Disable()
	w.statusLabel.SetText("Stream closed")
}

// Clear resets the widget for a new stream.
func (w *StreamingInputWidget) Clear() {
	w.messageEntry.SetText("")
	w.messageEntry.Enable()
	_ = w.sentMessages.Set([]string{})
	w.totalSent = 0
	w.sentList.Refresh()
	w.sendBtn.Enable()
	w.finishBtn.Enable()
	w.statusLabel.SetText("Ready")
}

// GetCurrentMessage returns the current message text.
func (w *StreamingInputWidget) GetCurrentMessage() string {
	return w.messageEntry.Text
}

// SetCurrentMessage sets the current message text.
func (w *StreamingInputWidget) SetCurrentMessage(text string) {
	w.messageEntry.SetText(text)
}

// SetStatus updates the status display.
func (w *StreamingInputWidget) SetStatus(status string) {
	w.statusLabel.SetText(status)
}

// DisableSendControls disables all send controls.
func (w *StreamingInputWidget) DisableSendControls() {
	w.sendBtn.Disable()
	w.finishBtn.Disable()
	w.messageEntry.Disable()
}

// updateStatus updates the status with message count.
func (w *StreamingInputWidget) updateStatus() {
	sentVisible := w.sentMessages.Length()
	sentStr := fmt.Sprintf("%d", sentVisible)
	if w.totalSent > sentVisible {
		sentStr = fmt.Sprintf("%d of %d", sentVisible, w.totalSent)
	}
	w.statusLabel.SetText(fmt.Sprintf("Sent: %s messages", sentStr))
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

	// Buttons at bottom - side by side
	buttonBox := container.NewHBox(
		w.sendBtn,
		w.finishBtn,
	)

	// Full layout: status at top, buttons at bottom, split in center
	content := container.NewBorder(
		container.NewVBox(w.statusLabel, widget.NewSeparator()), // top (status)
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
