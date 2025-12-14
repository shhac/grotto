package components

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// NewCollapsibleSection creates a collapsible section using Fyne's Accordion.
// The section starts collapsed by default to save vertical space.
func NewCollapsibleSection(title string, content fyne.CanvasObject) *widget.Accordion {
	accordion := widget.NewAccordion(
		widget.NewAccordionItem(title, content),
	)
	// Start collapsed to save space
	accordion.Close(0)
	return accordion
}

// NewExpandedCollapsibleSection creates a collapsible section that starts expanded.
func NewExpandedCollapsibleSection(title string, content fyne.CanvasObject) *widget.Accordion {
	accordion := widget.NewAccordion(
		widget.NewAccordionItem(title, content),
	)
	// Start expanded
	accordion.Open(0)
	return accordion
}
