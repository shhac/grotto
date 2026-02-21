package components

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ModeTabs provides a Text/Form mode toggle using a horizontal RadioGroup.
// This visually distinguishes the mode switch from content-level AppTabs.
// It manages switching between two content views and notifies listeners of mode changes.
type ModeTabs struct {
	widget.BaseWidget

	modeSelect   *widget.RadioGroup
	textContent  fyne.CanvasObject
	formContent  fyne.CanvasObject
	contentStack *fyne.Container // container.NewStack — holds active content

	onModeChange func(mode string)
}

// NewModeTabs creates a new ModeTabs widget with the provided content for each mode.
// textContent is displayed when the "Text" option is selected.
// formContent is displayed when the "Form" option is selected.
func NewModeTabs(textContent, formContent fyne.CanvasObject) *ModeTabs {
	m := &ModeTabs{
		textContent: textContent,
		formContent: formContent,
	}

	m.modeSelect = widget.NewRadioGroup([]string{"Text", "Form"}, func(selected string) {
		mode := strings.ToLower(selected)
		m.updateContent(mode)
		if m.onModeChange != nil {
			m.onModeChange(mode)
		}
	})
	m.modeSelect.Horizontal = true
	m.modeSelect.Selected = "Text"

	// Stack container shows the active mode's content
	m.contentStack = container.NewStack(textContent)

	m.ExtendBaseWidget(m)
	return m
}

// SetOnModeChange sets the callback that is invoked when the mode changes.
// The callback receives "text" or "form" as the mode parameter.
func (m *ModeTabs) SetOnModeChange(fn func(mode string)) {
	m.onModeChange = fn
}

// SetMode programmatically switches to the specified mode.
// Valid modes are "text" and "form".
// Does nothing if already on the requested mode (avoids triggering callback redundantly).
func (m *ModeTabs) SetMode(mode string) {
	if m.GetMode() == mode {
		return
	}

	switch mode {
	case "text":
		m.modeSelect.SetSelected("Text")
	case "form":
		m.modeSelect.SetSelected("Form")
	}
}

// GetMode returns the currently selected mode ("text" or "form").
func (m *ModeTabs) GetMode() string {
	if m.modeSelect.Selected == "" {
		return "text"
	}
	return strings.ToLower(m.modeSelect.Selected)
}

// updateContent swaps the visible content in the stack.
func (m *ModeTabs) updateContent(mode string) {
	switch mode {
	case "text":
		m.contentStack.Objects = []fyne.CanvasObject{m.textContent}
	case "form":
		m.contentStack.Objects = []fyne.CanvasObject{m.formContent}
	}
	m.contentStack.Refresh()
}

// CreateRenderer implements fyne.Widget.
func (m *ModeTabs) CreateRenderer() fyne.WidgetRenderer {
	content := container.NewBorder(m.modeSelect, nil, nil, nil, m.contentStack)
	return widget.NewSimpleRenderer(content)
}

// MinSize implements fyne.Widget.
func (m *ModeTabs) MinSize() fyne.Size {
	return m.BaseWidget.MinSize()
}
