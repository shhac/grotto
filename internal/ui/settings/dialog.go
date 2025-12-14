package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
)

// ShowTLSDialog displays a dialog for configuring TLS settings
func ShowTLSDialog(window fyne.Window, currentSettings domain.TLSSettings, onSave func(domain.TLSSettings)) {
	// Create TLS config widget
	tlsWidget := NewTLSConfig(window)
	tlsWidget.SetConfig(currentSettings)

	// Create dialog variable that will be set later
	var dlg dialog.Dialog

	// Add Save button
	saveBtn := widget.NewButton("Save", func() {
		cfg := tlsWidget.GetConfig()
		onSave(cfg)
		dlg.Hide()
	})

	cancelBtn := widget.NewButton("Cancel", func() {
		dlg.Hide()
	})

	buttons := container.NewHBox(
		saveBtn,
		cancelBtn,
	)

	content := container.NewBorder(nil, buttons, nil, nil, tlsWidget.container)

	dlg = dialog.NewCustom("TLS Configuration", "Close", content, window)
	dlg.Resize(fyne.NewSize(600, 500))
	dlg.Show()
}
