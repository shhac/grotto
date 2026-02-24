package workspace

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// ShowDeleteConfirm shows a confirmation dialog before deleting a workspace
func ShowDeleteConfirm(parent fyne.Window, name string, onConfirm func()) {
	dialog.ShowConfirm("Delete Workspace",
		"Are you sure you want to delete workspace '"+name+"'? This cannot be undone.",
		func(confirmed bool) {
			if confirmed {
				onConfirm()
			}
		},
		parent,
	)
}

// ShowErrorDialog shows an error message dialog
func ShowErrorDialog(parent fyne.Window, message string) {
	dialog.ShowError(errors.New(message), parent)
}

// ShowInfoDialog shows an information dialog
func ShowInfoDialog(parent fyne.Window, title, message string) {
	dialog.ShowInformation(title, message, parent)
}
