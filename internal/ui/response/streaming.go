package response

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	maxStreamMessages = 1000
	evictionBatch     = 200
)

// StreamingMessagesWidget displays streaming RPC messages as they arrive.
type StreamingMessagesWidget struct {
	widget.BaseWidget

	window        fyne.Window
	messages      binding.UntypedList // []string (JSON messages)
	messageList   *widget.List
	autoScroll    bool
	totalReceived int // total messages received (including evicted)

	// Status section
	statusLabel     *widget.Label
	stopBtn         *widget.Button
	copyAllBtn      *widget.Button
	autoScrollCheck *widget.Check
	statusBox       *fyne.Container

	// Main container
	container *fyne.Container

	// Callbacks
	onStop func()
}

// NewStreamingMessagesWidget creates a new streaming messages widget.
func NewStreamingMessagesWidget(window fyne.Window) *StreamingMessagesWidget {
	w := &StreamingMessagesWidget{
		window:     window,
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

	// Copy all button
	w.copyAllBtn = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		all, err := w.messages.Get()
		if err != nil || len(all) == 0 {
			return
		}
		var msgs []string
		for _, item := range all {
			if s, ok := item.(string); ok {
				msgs = append(msgs, s)
			}
		}
		w.window.Clipboard().SetContent(strings.Join(msgs, "\n"))
	})

	// Auto-scroll toggle
	w.autoScrollCheck = widget.NewCheck("Auto-scroll", func(checked bool) {
		w.autoScroll = checked
		if checked {
			w.messageList.ScrollToBottom()
		}
	})
	w.autoScrollCheck.SetChecked(true)

	// Status box (label + controls)
	w.statusBox = container.NewBorder(
		nil,
		nil,
		nil,
		container.NewHBox(w.autoScrollCheck, w.copyAllBtn, w.stopBtn),
		w.statusLabel,
	)

	// Message list with syntax-highlighted JSON
	w.messageList = widget.NewListWithData(
		w.messages,
		func() fyne.CanvasObject {
			rt := widget.NewRichText()
			rt.Wrapping = fyne.TextWrapBreak
			return rt
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			rt := obj.(*widget.RichText)
			if strItem, ok := item.(binding.String); ok {
				val, _ := strItem.Get()
				rt.Segments = HighlightJSON(val)
				rt.Refresh()
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
	w.messages.Append(jsonStr)
	w.totalReceived++

	// Evict oldest messages if over cap
	count := w.messages.Length()
	if count > maxStreamMessages {
		all, err := w.messages.Get()
		if err == nil && len(all) > maxStreamMessages {
			_ = w.messages.Set(all[evictionBatch:])
			count = w.messages.Length()
		}
	}

	// Update status
	if w.totalReceived > count {
		w.statusLabel.SetText(fmt.Sprintf("Streaming... (showing %d of %d messages)", count, w.totalReceived))
	} else {
		w.statusLabel.SetText(fmt.Sprintf("Streaming... (%d messages)", count))
	}

	// Auto-scroll to latest message if enabled
	if w.autoScroll {
		w.messageList.ScrollToBottom()
	}
}

// SetStatus updates the status label with a custom message.
func (w *StreamingMessagesWidget) SetStatus(status string) {
	w.statusLabel.SetText(status)
}

// Clear removes all messages from the list.
func (w *StreamingMessagesWidget) Clear() {
	_ = w.messages.Set([]interface{}{})
	w.totalReceived = 0
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
