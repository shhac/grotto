package bidi

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/ui/response"
	"github.com/shhac/grotto/internal/ui/streamconst"
)

// BidiStreamPanel provides UI for bidirectional streaming RPCs.
// It displays sent and received messages in a split view, allowing the user
// to send multiple messages while simultaneously receiving responses.
type BidiStreamPanel struct {
	widget.BaseWidget

	window fyne.Window

	// Send side (left)
	messageEntry *widget.Entry      // Current message to send
	sentList     *widget.List       // List of sent messages
	sentMessages binding.StringList // Binding for sent messages

	sendBtn      *widget.Button // Send current message
	closeSendBtn *widget.Button // Close send stream
	abortBtn     *widget.Button // Abort entire stream (cancel context)

	// Receive side (right)
	receivedList     *widget.List        // List of received messages
	receivedMessages binding.UntypedList // Binding for received messages
	autoScroll       bool
	autoScrollCheck  *widget.Check

	// Copy buttons
	copySentBtn     *widget.Button
	copyReceivedBtn *widget.Button

	// Counters (including evicted messages)
	totalSent     int
	totalReceived int

	// Status
	statusLabel *widget.Label

	// Main container
	container *fyne.Container

	// Callbacks
	onSend      func(json string) // Callback when Send is clicked
	onCloseSend func()            // Callback when Close Send is clicked
	onAbort     func()            // Callback when Abort Stream is clicked
}

// NewBidiStreamPanel creates a new bidirectional streaming panel.
func NewBidiStreamPanel(window fyne.Window) *BidiStreamPanel {
	p := &BidiStreamPanel{
		window:           window,
		sentMessages:     binding.NewStringList(),
		receivedMessages: binding.NewUntypedList(),
		autoScroll:       true,
	}
	p.ExtendBaseWidget(p)
	p.initializeComponents()
	return p
}

// initializeComponents creates all UI components.
func (p *BidiStreamPanel) initializeComponents() {
	// Message entry - multiline JSON editor
	p.messageEntry = widget.NewMultiLineEntry()
	p.messageEntry.SetPlaceHolder(`{"field": "value"}`)
	p.messageEntry.Wrapping = fyne.TextWrapWord

	// List of sent messages (syntax highlighted)
	p.sentList = widget.NewList(
		func() int {
			return p.sentMessages.Length()
		},
		func() fyne.CanvasObject {
			rt := widget.NewRichText()
			rt.Wrapping = fyne.TextWrapBreak
			return rt
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			rt := obj.(*widget.RichText)
			msg, _ := p.sentMessages.GetValue(id)
			rt.Segments = response.HighlightJSON(msg)
			rt.Refresh()
		},
	)

	// List of received messages (syntax highlighted)
	p.receivedList = widget.NewListWithData(
		p.receivedMessages,
		func() fyne.CanvasObject {
			rt := widget.NewRichText()
			rt.Wrapping = fyne.TextWrapBreak
			return rt
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			rt := obj.(*widget.RichText)
			if strItem, ok := item.(binding.String); ok {
				val, _ := strItem.Get()
				rt.Segments = response.HighlightJSON(val)
				rt.Refresh()
			}
		},
	)

	// Buttons
	p.sendBtn = widget.NewButton("Send", func() {
		p.handleSend()
	})

	p.closeSendBtn = widget.NewButton("Close Send", func() {
		p.handleCloseSend()
	})
	p.closeSendBtn.Importance = widget.WarningImportance

	p.abortBtn = widget.NewButton("Abort Stream", func() {
		p.handleAbort()
	})
	p.abortBtn.Importance = widget.DangerImportance

	// Copy buttons
	p.copySentBtn = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		all, _ := p.sentMessages.Get()
		if len(all) > 0 {
			p.window.Clipboard().SetContent(strings.Join(all, "\n"))
		}
	})

	p.copyReceivedBtn = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		all, err := p.receivedMessages.Get()
		if err != nil || len(all) == 0 {
			return
		}
		var msgs []string
		for _, item := range all {
			if s, ok := item.(string); ok {
				msgs = append(msgs, s)
			}
		}
		p.window.Clipboard().SetContent(strings.Join(msgs, "\n"))
	})

	// Auto-scroll toggle
	p.autoScrollCheck = widget.NewCheck("Auto-scroll", func(checked bool) {
		p.autoScroll = checked
		if checked {
			p.receivedList.ScrollToBottom()
		}
	})
	p.autoScrollCheck.SetChecked(true)

	// Status label
	p.statusLabel = widget.NewLabel("Ready")

	// Build layout
	p.buildLayout()
}

// buildLayout constructs the split view layout.
func (p *BidiStreamPanel) buildLayout() {
	// Left side: Send section
	sentCountLabel := widget.NewLabel("Sent:")
	sentCountLabel.TextStyle = fyne.TextStyle{Bold: true}

	sentSection := container.NewBorder(
		container.NewBorder(nil, nil, sentCountLabel, p.copySentBtn),
		nil, nil, nil,
		p.sentList,
	)

	nextLabel := widget.NewLabel("Next message:")
	nextLabel.TextStyle = fyne.TextStyle{Bold: true}

	messageSection := container.NewBorder(
		nextLabel,
		nil, nil, nil,
		p.messageEntry,
	)

	sendButtons := container.NewHBox(
		p.sendBtn,
		layout.NewSpacer(),
		p.closeSendBtn,
		p.abortBtn,
	)

	leftPanel := container.NewBorder(
		nil,         // top
		sendButtons, // bottom (buttons)
		nil, nil,    // left, right
		container.NewVSplit(
			sentSection,    // top half (sent messages)
			messageSection, // bottom half (next message)
		),
	)

	// Right side: Receive section
	receivedLabel := widget.NewLabel("Received:")
	receivedLabel.TextStyle = fyne.TextStyle{Bold: true}

	rightPanel := container.NewBorder(
		container.NewBorder(nil, nil, receivedLabel, container.NewHBox(p.autoScrollCheck, p.copyReceivedBtn)),
		nil, nil, nil,
		p.receivedList,
	)

	// Main split: left (send) and right (receive)
	mainSplit := container.NewHSplit(
		leftPanel,
		rightPanel,
	)
	mainSplit.SetOffset(0.5) // 50/50 split

	// Wrap with status at top
	p.container = container.NewBorder(
		container.NewVBox(
			p.statusLabel,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		mainSplit,
	)
}

// SetOnSend sets the callback for when a message is sent.
func (p *BidiStreamPanel) SetOnSend(fn func(json string)) {
	p.onSend = fn
}

// SetOnCloseSend sets the callback for when the send side is closed.
func (p *BidiStreamPanel) SetOnCloseSend(fn func()) {
	p.onCloseSend = fn
}

// SetOnAbort sets the callback for when the stream is aborted.
func (p *BidiStreamPanel) SetOnAbort(fn func()) {
	p.onAbort = fn
}

// handleSend sends the current message and adds it to the sent list.
func (p *BidiStreamPanel) handleSend() {
	if p.onSend == nil {
		return
	}

	msg := p.messageEntry.Text
	if msg == "" {
		return // Don't send empty messages
	}

	// Call the callback
	p.onSend(msg)

	// Add to sent messages list
	_ = p.sentMessages.Append(msg)
	p.totalSent++

	// Evict oldest if over cap
	if count := p.sentMessages.Length(); count > streamconst.MaxStreamMessages {
		all, err := p.sentMessages.Get()
		if err == nil && len(all) > streamconst.MaxStreamMessages {
			_ = p.sentMessages.Set(all[streamconst.EvictionBatch:])
		}
	}

	// Clear the entry for next message
	p.messageEntry.SetText("")

	// Refresh the list
	p.sentList.Refresh()

	// Update status
	p.updateStatus()
}

// handleCloseSend closes the send side of the stream.
func (p *BidiStreamPanel) handleCloseSend() {
	if p.onCloseSend == nil {
		return
	}

	p.onCloseSend()

	// Disable send controls
	p.sendBtn.Disable()
	p.closeSendBtn.Disable()
	p.messageEntry.Disable()

	// Update status
	p.statusLabel.SetText("Send closed")
}

// handleAbort fully cancels the stream (both send and receive).
func (p *BidiStreamPanel) handleAbort() {
	if p.onAbort == nil {
		return
	}

	p.onAbort()
	p.sendBtn.Disable()
	p.closeSendBtn.Disable()
	p.abortBtn.Disable()
	p.messageEntry.Disable()
	p.statusLabel.SetText("Stream aborted")
}

// AddReceived adds a received message to the list (thread-safe via bindings).
func (p *BidiStreamPanel) AddReceived(json string) {
	p.receivedMessages.Append(json)
	p.totalReceived++

	// Evict oldest if over cap
	if count := p.receivedMessages.Length(); count > streamconst.MaxStreamMessages {
		all, err := p.receivedMessages.Get()
		if err == nil && len(all) > streamconst.MaxStreamMessages {
			_ = p.receivedMessages.Set(all[streamconst.EvictionBatch:])
		}
	}

	p.receivedList.Refresh()

	// Auto-scroll to latest message
	if p.autoScroll {
		p.receivedList.ScrollToBottom()
	}

	// Update status
	p.updateStatus()
}

// SetStatus updates the status display.
func (p *BidiStreamPanel) SetStatus(status string) {
	p.statusLabel.SetText(status)
}

// updateStatus updates the status with message counts.
func (p *BidiStreamPanel) updateStatus() {
	sentVisible := p.sentMessages.Length()
	recvVisible := p.receivedMessages.Length()

	sentStr := fmt.Sprintf("%d", sentVisible)
	if p.totalSent > sentVisible {
		sentStr = fmt.Sprintf("%d of %d", sentVisible, p.totalSent)
	}
	recvStr := fmt.Sprintf("%d", recvVisible)
	if p.totalReceived > recvVisible {
		recvStr = fmt.Sprintf("%d of %d", recvVisible, p.totalReceived)
	}

	p.statusLabel.SetText(fmt.Sprintf("Sent: %s | Received: %s", sentStr, recvStr))
}

// Clear resets the panel for a new stream.
func (p *BidiStreamPanel) Clear() {
	p.messageEntry.SetText("")
	p.messageEntry.Enable()

	_ = p.sentMessages.Set([]string{})
	p.totalSent = 0
	p.sentList.Refresh()

	_ = p.receivedMessages.Set([]interface{}{})
	p.totalReceived = 0
	p.receivedList.Refresh()

	p.sendBtn.Enable()
	p.closeSendBtn.Enable()
	p.abortBtn.Enable()

	p.statusLabel.SetText("Ready")
}

// DisableSendControls disables the send controls (when stream errors).
func (p *BidiStreamPanel) DisableSendControls() {
	p.sendBtn.Disable()
	p.closeSendBtn.Disable()
	p.abortBtn.Disable()
	p.messageEntry.Disable()
}

// CreateRenderer implements fyne.Widget.
func (p *BidiStreamPanel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.container)
}

// MinSize implements fyne.Widget.
func (p *BidiStreamPanel) MinSize() fyne.Size {
	return fyne.NewSize(800, 500)
}
