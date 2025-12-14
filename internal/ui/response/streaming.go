package response

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

// StreamingMessagesWidget displays streaming RPC messages as they arrive.
type StreamingMessagesWidget struct {
	widget.BaseWidget

	messages    binding.UntypedList // []string (JSON messages)
	messageList *widget.List
	autoScroll  bool

	// Status section
	statusLabel *widget.Label
	stopBtn     *widget.Button
	statusBox   *fyne.Container

	// Main container
	container *fyne.Container

	// Callbacks
	onStop func()
}

// NewStreamingMessagesWidget creates a new streaming messages widget.
func NewStreamingMessagesWidget() *StreamingMessagesWidget {
	w := &StreamingMessagesWidget{
		messages:   binding.NewUntypedList(),
		autoScroll: true,
	}
	w.ExtendBaseWidget(w)
	w.initializeComponents()
	return w
}

// initializeComponents creates all UI components.
func (w *StreamingMessagesWidget) initializeComponents() {
	// Status label
	w.statusLabel = widget.NewLabel("Ready")

	// Stop button (styled as danger to make it prominent)
	w.stopBtn = widget.NewButton("Abort Stream", func() {
		if w.onStop != nil {
			w.onStop()
		}
	})
	w.stopBtn.Importance = widget.DangerImportance
	w.stopBtn.Disable() // Disabled by default until streaming starts

	// Status box (label + stop button)
	w.statusBox = container.NewBorder(
		nil,
		nil,
		nil,
		w.stopBtn,
		w.statusLabel,
	)

	// Message list
	w.messageList = widget.NewListWithData(
		w.messages,
		func() fyne.CanvasObject {
			// Template: multiline entry for JSON display
			entry := widget.NewMultiLineEntry()
			entry.Wrapping = fyne.TextWrapWord
			entry.Disable() // Read-only
			return entry
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			// Update the entry with the message text
			entry := obj.(*widget.Entry)
			if strItem, ok := item.(binding.String); ok {
				entry.Bind(strItem)
			}
		},
	)

	// Header for streaming section
	header := widget.NewLabel("Streaming Messages")
	header.TextStyle = fyne.TextStyle{Bold: true}

	// Main container with status at top and clear visual hierarchy
	w.container = container.NewBorder(
		container.NewVBox(
			header,
			widget.NewSeparator(),
			w.statusBox,
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		w.messageList,
	)
}

// AddMessage appends a message to the list (thread-safe).
// This should be called from a goroutine using fyne.Do() wrapper.
func (w *StreamingMessagesWidget) AddMessage(jsonStr string) {
	// Append to messages list
	w.messages.Append(jsonStr)

	// Update status
	count := w.messages.Length()
	w.statusLabel.SetText(fmt.Sprintf("Streaming... (%d messages)", count))

	// Auto-scroll to latest message if enabled
	if w.autoScroll {
		// Scroll to the latest item
		w.messageList.ScrollToBottom()
	}
}

// SetStatus updates the status label with a custom message.
func (w *StreamingMessagesWidget) SetStatus(status string) {
	w.statusLabel.SetText(status)
}

// Clear removes all messages from the list.
func (w *StreamingMessagesWidget) Clear() {
	// Clear all items from the list
	for w.messages.Length() > 0 {
		_ = w.messages.Set([]interface{}{})
	}
	w.messageList.Refresh()
	w.statusLabel.SetText("Ready")
}

// SetOnStop sets the callback for the stop button.
func (w *StreamingMessagesWidget) SetOnStop(fn func()) {
	w.onStop = fn
}

// EnableStopButton enables the stop button (call when streaming starts).
func (w *StreamingMessagesWidget) EnableStopButton() {
	w.stopBtn.Enable()
}

// DisableStopButton disables the stop button (call when streaming completes).
func (w *StreamingMessagesWidget) DisableStopButton() {
	w.stopBtn.Disable()
}

// CreateRenderer implements fyne.Widget.
func (w *StreamingMessagesWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.container)
}

// MinSize implements fyne.Widget.
func (w *StreamingMessagesWidget) MinSize() fyne.Size {
	return fyne.NewSize(400, 300)
}
