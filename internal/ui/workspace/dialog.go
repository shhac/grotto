package workspace

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// ShowSaveDialog shows a dialog to save a new workspace
func ShowSaveDialog(parent fyne.Window, onSave func(name string)) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Enter workspace name")

	dialog.ShowForm("Save Workspace", "Save", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Name", entry),
		},
		func(confirmed bool) {
			if confirmed && entry.Text != "" {
				onSave(entry.Text)
			}
		},
		parent,
	)
}

// ShowLoadDialog shows a dialog to load an existing workspace
func ShowLoadDialog(parent fyne.Window, workspaces []string, onLoad func(name string)) {
	if len(workspaces) == 0 {
		dialog.ShowInformation("No Workspaces", "No saved workspaces found", parent)
		return
	}

	selectWidget := widget.NewSelect(workspaces, func(value string) {})
	if len(workspaces) > 0 {
		selectWidget.SetSelected(workspaces[0])
	}

	dialog.ShowForm("Load Workspace", "Load", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Workspace", selectWidget),
		},
		func(confirmed bool) {
			if confirmed && selectWidget.Selected != "" {
				onLoad(selectWidget.Selected)
			}
		},
		parent,
	)
}

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
