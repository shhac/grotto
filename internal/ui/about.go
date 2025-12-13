package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// ShowAboutDialog displays information about the Grotto application.
func ShowAboutDialog(parent fyne.Window) {
	content := container.NewVBox(
		widget.NewLabelWithStyle("Grotto", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("A permissive, user-friendly gRPC client"),
		widget.NewLabel("Version 0.1.0"),
		widget.NewSeparator(),
		widget.NewLabel("Built with Fyne and Go"),
	)
	dialog.ShowCustom("About Grotto", "Close", content, parent)
}
