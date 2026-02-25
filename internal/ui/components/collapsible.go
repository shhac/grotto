package components

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// TreeSection is a collapsible section with filled triangle ▶/▼ disclosure icons
// that indicate nested structure, rather than accordion-style ⌃/⌄ icons.
type TreeSection struct {
	widget.BaseWidget

	expanded bool
	content  fyne.CanvasObject
	icon     *widget.Icon
	wrapper  *fyne.Container
}

// NewCollapsibleSection creates a tree-style collapsible section, initially collapsed.
func NewCollapsibleSection(title string, content fyne.CanvasObject) *TreeSection {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return newTreeSection(titleLabel, content, false)
}

// NewCollapsibleSectionWithHint creates a collapsible section with a subdued type hint.
func NewCollapsibleSectionWithHint(title, hint string, content fyne.CanvasObject) *TreeSection {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hintLabel := NewHintLabel(hint)
	titleRow := container.NewHBox(titleLabel, hintLabel)
	return newTreeSection(titleRow, content, false)
}

// NewExpandedCollapsibleSection creates a tree-style collapsible section, initially expanded.
func NewExpandedCollapsibleSection(title string, content fyne.CanvasObject) *TreeSection {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return newTreeSection(titleLabel, content, true)
}

func newTreeSection(titleContent fyne.CanvasObject, content fyne.CanvasObject, expanded bool) *TreeSection {
	ts := &TreeSection{
		expanded: expanded,
		content:  content,
	}

	if expanded {
		ts.icon = widget.NewIcon(theme.MenuDropDownIcon())
	} else {
		ts.icon = widget.NewIcon(theme.MenuExpandIcon())
	}

	header := newTappableRow(
		container.NewHBox(ts.icon, titleContent),
		func() { ts.Toggle() },
	)

	ts.wrapper = container.NewVBox(header, content)

	if !expanded {
		content.Hide()
	}

	ts.ExtendBaseWidget(ts)
	return ts
}

// Toggle flips the expanded/collapsed state.
func (t *TreeSection) Toggle() {
	if t.expanded {
		t.Close()
	} else {
		t.Open()
	}
}

// Open expands the section.
func (t *TreeSection) Open() {
	t.expanded = true
	t.icon.SetResource(theme.MenuDropDownIcon())
	t.content.Show()
	t.Refresh()
}

// Close collapses the section.
func (t *TreeSection) Close() {
	t.expanded = false
	t.icon.SetResource(theme.MenuExpandIcon())
	t.content.Hide()
	t.Refresh()
}

// IsExpanded returns whether the section is currently expanded.
func (t *TreeSection) IsExpanded() bool {
	return t.expanded
}

// CreateRenderer implements fyne.Widget.
func (t *TreeSection) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.wrapper)
}

// tappableRow is a tappable container that triggers a callback on tap.
type tappableRow struct {
	widget.BaseWidget
	child    fyne.CanvasObject
	onTapped func()
}

func newTappableRow(child fyne.CanvasObject, onTapped func()) *tappableRow {
	t := &tappableRow{child: child, onTapped: onTapped}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappableRow) Tapped(_ *fyne.PointEvent) {
	if t.onTapped != nil {
		t.onTapped()
	}
}

func (t *tappableRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.child)
}
