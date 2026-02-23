package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/shhac/grotto/internal/domain"
)

// ShowTLSDialog displays a dialog for configuring TLS settings
func ShowTLSDialog(window fyne.Window, currentSettings domain.TLSSettings, onSave func(domain.TLSSettings)) {
	tlsWidget := NewTLSConfig(window)
	tlsWidget.SetConfig(currentSettings)

	dlg := dialog.NewCustomConfirm("TLS Configuration", "Save", "Cancel", tlsWidget.container, func(save bool) {
		if save {
			onSave(tlsWidget.GetConfig())
		}
	}, window)
	dlg.Resize(fyne.NewSize(600, 500))
	dlg.Show()
}
