package components

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ModeTabs provides a Text/Form mode toggle using Fyne's AppTabs.
// It manages switching between two content views and notifies listeners of mode changes.
type ModeTabs struct {
	widget.BaseWidget

	tabs        *container.AppTabs
	textTab     *container.TabItem
	formTab     *container.TabItem

	textContent fyne.CanvasObject
	formContent fyne.CanvasObject

	onModeChange func(mode string)
}

// NewModeTabs creates a new ModeTabs widget with the provided content for each mode.
// textContent is displayed when the "Text" tab is selected.
// formContent is displayed when the "Form" tab is selected.
func NewModeTabs(textContent, formContent fyne.CanvasObject) *ModeTabs {
	m := &ModeTabs{
		textContent: textContent,
		formContent: formContent,
	}

	// Create tab items
	m.textTab = container.NewTabItem("Text", textContent)
	m.formTab = container.NewTabItem("Form", formContent)

	// Create AppTabs container
	m.tabs = container.NewAppTabs(m.textTab, m.formTab)

	// Listen for tab selection changes
	m.tabs.OnSelected = func(tab *container.TabItem) {
		mode := m.getTabMode(tab)
		if m.onModeChange != nil {
			m.onModeChange(mode)
		}
	}

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
func (m *ModeTabs) SetMode(mode string) {
	switch mode {
	case "text":
		m.tabs.Select(m.textTab)
	case "form":
		m.tabs.Select(m.formTab)
	}
}

// GetMode returns the currently selected mode ("text" or "form").
func (m *ModeTabs) GetMode() string {
	selected := m.tabs.Selected()
	return m.getTabMode(selected)
}

// getTabMode converts a tab item to its corresponding mode string.
func (m *ModeTabs) getTabMode(tab *container.TabItem) string {
	if tab == m.textTab {
		return "text"
	}
	return "form"
}

// CreateRenderer implements fyne.Widget.
func (m *ModeTabs) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(m.tabs)
}

// MinSize implements fyne.Widget.
func (m *ModeTabs) MinSize() fyne.Size {
	return m.tabs.MinSize()
}
