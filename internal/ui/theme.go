package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ThemePreferenceKey is the key used to store theme preference
const ThemePreferenceKey = "appTheme"

// forcedVariant wraps a theme to force a specific variant (light/dark)
type forcedVariant struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

// Color returns the color for the forced variant, ignoring the passed variant
func (f *forcedVariant) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return f.Theme.Color(name, f.variant)
}

// ApplyTheme sets the application theme based on the mode
// mode can be "dark", "light", or "system" (default)
func ApplyTheme(a fyne.App, mode string) {
	switch mode {
	case "dark":
		a.Settings().SetTheme(&forcedVariant{
			Theme:   theme.DefaultTheme(),
			variant: theme.VariantDark,
		})
	case "light":
		a.Settings().SetTheme(&forcedVariant{
			Theme:   theme.DefaultTheme(),
			variant: theme.VariantLight,
		})
	default: // "system"
		a.Settings().SetTheme(theme.DefaultTheme())
	}
}

// LoadThemePreference loads and applies the saved theme preference
func LoadThemePreference(a fyne.App) {
	mode := a.Preferences().StringWithFallback(ThemePreferenceKey, "system")
	ApplyTheme(a, mode)
}

// SaveThemePreference saves and applies the theme preference
func SaveThemePreference(a fyne.App, mode string) {
	a.Preferences().SetString(ThemePreferenceKey, mode)
	ApplyTheme(a, mode)
}

// CreateThemeSelector creates a widget for selecting the theme
func CreateThemeSelector(a fyne.App) *widget.Select {
	selector := widget.NewSelect(
		[]string{"System Default", "Light", "Dark"},
		func(selected string) {
			var mode string
			switch selected {
			case "Dark":
				mode = "dark"
			case "Light":
				mode = "light"
			default:
				mode = "system"
			}
			SaveThemePreference(a, mode)
		},
	)

	// Set initial selection from preferences
	saved := a.Preferences().StringWithFallback(ThemePreferenceKey, "system")
	switch saved {
	case "dark":
		selector.SetSelected("Dark")
	case "light":
		selector.SetSelected("Light")
	default:
		selector.SetSelected("System Default")
	}
	return selector
}
