package components

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Compile-time interface check.
var _ desktop.Hoverable = (*HintLabel)(nil)

const hintMaxRunes = 8

// HintLabel displays a subdued type hint that truncates long text with "…"
// and shows the full text in a popup on hover.
type HintLabel struct {
	widget.BaseWidget

	fullText string
	label    *widget.Label
	popup    *widget.PopUp
}

// NewHintLabel creates a LowImportance label that truncates text longer than
// hintMaxRunes and reveals the full text on mouse hover.
func NewHintLabel(text string) *HintLabel {
	h := &HintLabel{fullText: text}
	h.label = widget.NewLabel(truncateRunes(text, hintMaxRunes))
	h.label.Importance = widget.LowImportance
	h.ExtendBaseWidget(h)
	return h
}

// truncateRunes returns s unchanged if it has at most max runes,
// otherwise truncates to max-1 runes and appends "…".
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// MouseIn shows a tooltip popup with the full text when the hint is truncated.
func (h *HintLabel) MouseIn(_ *desktop.MouseEvent) {
	if !h.needsTooltip() {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(h)
	if c == nil {
		return
	}
	tip := widget.NewLabel(h.fullText)
	h.popup = widget.NewPopUp(tip, c)
	h.popup.ShowAtRelativePosition(fyne.NewPos(0, h.Size().Height), h)
}

// MouseMoved is required by desktop.Hoverable but needs no action.
func (h *HintLabel) MouseMoved(_ *desktop.MouseEvent) {}

// MouseOut hides and discards the tooltip popup.
func (h *HintLabel) MouseOut() {
	if h.popup != nil {
		h.popup.Hide()
		h.popup = nil
	}
}

func (h *HintLabel) needsTooltip() bool {
	return len([]rune(h.fullText)) > hintMaxRunes
}

// CreateRenderer implements fyne.Widget.
func (h *HintLabel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.label)
}
