package settings

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// Preference keys (must match the constants used elsewhere in the app).
const (
	PrefRequestTimeout = "requestTimeout"
	PrefTheme          = "appTheme"
)

// PreferencesCallbacks provides hooks for the preferences dialog to apply changes.
type PreferencesCallbacks struct {
	OnThemeChange func(mode string) // Called with "system", "dark", or "light"
}

// ShowPreferencesDialog displays the unified preferences dialog with General and Appearance tabs.
func ShowPreferencesDialog(a fyne.App, window fyne.Window, callbacks PreferencesCallbacks) {
	prefs := a.Preferences()

	// --- General tab ---

	currentTimeout := prefs.FloatWithFallback(PrefRequestTimeout, 30)
	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetText(strconv.FormatFloat(currentTimeout, 'f', -1, 64))

	generalTab := container.NewTabItem("General", container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Request Timeout (seconds)", timeoutEntry),
		),
		widget.NewLabel("Timeout for unary RPC requests. Streaming RPCs are not affected."),
	))

	// --- Appearance tab ---

	themeSelector := widget.NewSelect(
		[]string{"System Default", "Light", "Dark"},
		nil,
	)

	savedTheme := prefs.StringWithFallback(PrefTheme, "system")
	switch savedTheme {
	case "dark":
		themeSelector.SetSelected("Dark")
	case "light":
		themeSelector.SetSelected("Light")
	default:
		themeSelector.SetSelected("System Default")
	}

	appearanceTab := container.NewTabItem("Appearance", container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Theme", themeSelector),
		),
	))

	// --- Build dialog ---

	tabs := container.NewAppTabs(generalTab, appearanceTab)

	dlg := dialog.NewCustomConfirm("Preferences", "Save", "Cancel", tabs, func(save bool) {
		if !save {
			return
		}

		// Save timeout
		if val, err := strconv.ParseFloat(timeoutEntry.Text, 64); err == nil && val > 0 {
			prefs.SetFloat(PrefRequestTimeout, val)
		}

		// Save and apply theme
		var mode string
		switch themeSelector.Selected {
		case "Dark":
			mode = "dark"
		case "Light":
			mode = "light"
		default:
			mode = "system"
		}
		prefs.SetString(PrefTheme, mode)
		if callbacks.OnThemeChange != nil {
			callbacks.OnThemeChange(mode)
		}
	}, window)

	dlg.Resize(fyne.NewSize(500, 350))
	dlg.Show()
}
