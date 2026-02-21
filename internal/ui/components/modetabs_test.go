package components

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
)

func TestNewModeTabs(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	assert.NotNil(t, modeTabs, "ModeTabs should not be nil")
	assert.NotNil(t, modeTabs.tabs, "tabs should be initialized")
	assert.NotNil(t, modeTabs.textTab, "textTab should be initialized")
	assert.NotNil(t, modeTabs.formTab, "formTab should be initialized")
	assert.Equal(t, textContent, modeTabs.textContent)
	assert.Equal(t, formContent, modeTabs.formContent)
}

func TestModeTabs_GetMode_DefaultsToText(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	// Default mode should be "text" (first tab)
	mode := modeTabs.GetMode()
	assert.Equal(t, "text", mode, "default mode should be text")
}

func TestModeTabs_SetMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	tests := []struct {
		name         string
		setMode      string
		expectedMode string
	}{
		{
			name:         "switch to form mode",
			setMode:      "form",
			expectedMode: "form",
		},
		{
			name:         "switch back to text mode",
			setMode:      "text",
			expectedMode: "text",
		},
		{
			name:         "switch to form again",
			setMode:      "form",
			expectedMode: "form",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modeTabs.SetMode(tt.setMode)
			mode := modeTabs.GetMode()
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

func TestModeTabs_SetMode_NoOpWhenAlreadyOnMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	// Set to form mode
	modeTabs.SetMode("form")
	assert.Equal(t, "form", modeTabs.GetMode())

	// Set to form mode again (should be no-op)
	modeTabs.SetMode("form")
	assert.Equal(t, "form", modeTabs.GetMode())
}

func TestModeTabs_OnModeChange(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	// Track callback invocations
	callbackCalls := []string{}

	modeTabs.SetOnModeChange(func(mode string) {
		callbackCalls = append(callbackCalls, mode)
	})

	// Switch to form mode
	modeTabs.SetMode("form")
	assert.Len(t, callbackCalls, 1, "callback should be called once")
	assert.Equal(t, "form", callbackCalls[0])

	// Switch back to text mode
	modeTabs.SetMode("text")
	assert.Len(t, callbackCalls, 2, "callback should be called twice")
	assert.Equal(t, "text", callbackCalls[1])
}

func TestModeTabs_OnModeChange_NotCalledWhenAlreadyOnMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	callbackCount := 0
	modeTabs.SetOnModeChange(func(mode string) {
		callbackCount++
	})

	// Initially on text mode, set to text again (should be no-op)
	modeTabs.SetMode("text")
	assert.Equal(t, 0, callbackCount, "callback should not be called when already on mode")

	// Switch to form mode
	modeTabs.SetMode("form")
	assert.Equal(t, 1, callbackCount, "callback should be called once")

	// Set to form again (should be no-op)
	modeTabs.SetMode("form")
	assert.Equal(t, 1, callbackCount, "callback should not be called again")
}

func TestModeTabs_GetTabMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	tests := []struct {
		name     string
		tab      string
		expected string
	}{
		{
			name:     "text tab returns text",
			tab:      "textTab",
			expected: "text",
		},
		{
			name:     "form tab returns form",
			tab:      "formTab",
			expected: "form",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mode string
			if tt.tab == "textTab" {
				mode = modeTabs.getTabMode(modeTabs.textTab)
			} else {
				mode = modeTabs.getTabMode(modeTabs.formTab)
			}
			assert.Equal(t, tt.expected, mode)
		})
	}
}

func TestModeTabs_CreateRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	renderer := modeTabs.CreateRenderer()
	assert.NotNil(t, renderer, "renderer should not be nil")
}

func TestModeTabs_MinSize(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	minSize := modeTabs.MinSize()
	assert.Greater(t, minSize.Width, float32(0), "min width should be positive")
	assert.Greater(t, minSize.Height, float32(0), "min height should be positive")
}

func TestModeTabs_TabLabels(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	// Verify tab labels are correct
	assert.Equal(t, "Text", modeTabs.textTab.Text, "text tab should be labeled 'Text'")
	assert.Equal(t, "Form", modeTabs.formTab.Text, "form tab should be labeled 'Form'")
}

func TestModeTabs_InvalidMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	// Set to form mode first
	modeTabs.SetMode("form")
	assert.Equal(t, "form", modeTabs.GetMode())

	// Try to set an invalid mode (should be ignored)
	modeTabs.SetMode("invalid")
	// Should stay on form mode
	assert.Equal(t, "form", modeTabs.GetMode())
}

func TestModeTabs_ConcurrentModeChanges(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	textContent := widget.NewLabel("Text Mode Content")
	formContent := widget.NewLabel("Form Mode Content")

	modeTabs := NewModeTabs(textContent, formContent)

	// Rapidly switch modes multiple times
	for i := 0; i < 10; i++ {
		modeTabs.SetMode("form")
		modeTabs.SetMode("text")
	}

	// Should end on text mode
	assert.Equal(t, "text", modeTabs.GetMode())
}
