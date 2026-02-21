package browser

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// treeTheme is a custom theme that overrides tree expand/collapse icons
// while delegating all other theme queries to the parent theme
type treeTheme struct {
	parent fyne.Theme
}

// newTreeTheme creates a new tree theme that wraps the parent theme
func newTreeTheme(parent fyne.Theme) fyne.Theme {
	if parent == nil {
		parent = theme.DefaultTheme()
	}
	return &treeTheme{
		parent: parent,
	}
}

// Color delegates color queries to the parent theme
func (t *treeTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return t.parent.Color(name, variant)
}

// Font delegates font queries to the parent theme
func (t *treeTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.parent.Font(style)
}

// Icon overrides specific icons for tree navigation while delegating others
func (t *treeTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	switch name {
	case theme.IconNameNavigateNext:
		// Right chevron for collapsed state
		// Use the default navigate next icon (already a right-pointing chevron)
		return theme.NavigateNextIcon()
	case theme.IconNameMoveDown:
		// Down chevron for expanded state
		// Use menu dropdown icon for a clearer downward chevron
		return theme.MenuDropDownIcon()
	default:
		// Delegate all other icons to the parent theme
		return t.parent.Icon(name)
	}
}

// Size delegates size queries to the parent theme
func (t *treeTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.parent.Size(name)
}
