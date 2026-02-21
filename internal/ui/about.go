package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// Version is set at build time via ldflags:
//
//	go build -ldflags "-X github.com/shhac/grotto/internal/ui.Version=1.2.3"
var Version = "dev"

// ShowAboutDialog displays information about the Grotto application.
func ShowAboutDialog(parent fyne.Window) {
	content := container.NewVBox(
		widget.NewLabelWithStyle("Grotto", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("A permissive, user-friendly gRPC client"),
		widget.NewLabel("Version "+Version),
		widget.NewSeparator(),
		widget.NewLabel("Built with Fyne and Go"),
	)
	dialog.ShowCustom("About Grotto", "Close", content, parent)
}

// ShowShortcutDialog displays a reference of all keyboard shortcuts.
func ShowShortcutDialog(parent fyne.Window) {
	shortcuts := []struct{ action, key string }{
		{"Send Request", "\u2318 Return"},
		{"Save Workspace", "\u2318 S"},
		{"Load Workspace", "\u2318 O"},
		{"Focus Address Bar", "\u2318 K"},
		{"Clear Response", "\u2318 L"},
		{"Text Mode", "\u2318 1"},
		{"Form Mode", "\u2318 2"},
		{"Connect / Disconnect", "\u2318 \u21e7 C"},
		{"Cancel Operation", "Escape"},
	}

	grid := container.NewGridWithColumns(2)
	for _, s := range shortcuts {
		grid.Add(widget.NewLabel(s.action))
		grid.Add(widget.NewLabelWithStyle(s.key, fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true}))
	}

	dialog.ShowCustom("Keyboard Shortcuts", "Close", container.NewVScroll(grid), parent)
}
