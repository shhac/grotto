package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// setupKeyboardShortcuts configures all keyboard shortcuts for the main window
func (w *MainWindow) setupKeyboardShortcuts() {
	canvas := w.window.Canvas()

	// Cmd+Enter: Send request
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyReturn,
		Modifier: fyne.KeyModifierSuper, // Cmd on macOS, Win on Windows
	}, func(shortcut fyne.Shortcut) {
		w.logger.Debug("keyboard shortcut: send request")
		w.requestPanel.TriggerSend()
	})

	// Cmd+S: Save workspace
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierSuper,
	}, func(shortcut fyne.Shortcut) {
		w.logger.Debug("keyboard shortcut: save workspace")
		w.workspacePanel.TriggerSave()
	})

	// Cmd+O: Load workspace
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierSuper,
	}, func(shortcut fyne.Shortcut) {
		w.logger.Debug("keyboard shortcut: load workspace")
		w.workspacePanel.TriggerLoad()
	})

	// Cmd+K: Focus address bar
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyK,
		Modifier: fyne.KeyModifierSuper,
	}, func(shortcut fyne.Shortcut) {
		w.logger.Debug("keyboard shortcut: focus address bar")
		w.connectionBar.FocusAddress()
	})

	// Cmd+L: Clear response
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyL,
		Modifier: fyne.KeyModifierSuper,
	}, func(shortcut fyne.Shortcut) {
		w.logger.Debug("keyboard shortcut: clear response")
		w.responsePanel.ClearResponse()
	})

	// Cmd+1: Switch to Text mode
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.Key1,
		Modifier: fyne.KeyModifierSuper,
	}, func(shortcut fyne.Shortcut) {
		w.logger.Debug("keyboard shortcut: switch to text mode")
		w.requestPanel.SwitchToTextMode()
	})

	// Cmd+2: Switch to Form mode
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.Key2,
		Modifier: fyne.KeyModifierSuper,
	}, func(shortcut fyne.Shortcut) {
		w.logger.Debug("keyboard shortcut: switch to form mode")
		w.requestPanel.SwitchToFormMode()
	})

	// Escape: Cancel current operation (for streaming)
	canvas.SetOnTypedKey(func(key *fyne.KeyEvent) {
		if key.Name == fyne.KeyEscape {
			w.logger.Debug("keyboard shortcut: escape (cancel operation)")
			w.handleCancelOperation()
		}
	})

	w.logger.Info("keyboard shortcuts configured")
}

// handleCancelOperation cancels any active streaming operation
func (w *MainWindow) handleCancelOperation() {
	// Cancel bidi stream if active
	if w.bidiCancelFunc != nil {
		w.bidiCancelFunc()
		w.bidiCancelFunc = nil
		w.bidiStreamHandle = nil
		w.bidiPanel.SetStatus("Cancelled by user (Escape)")
		w.logger.Info("bidi stream cancelled by user")
		return
	}

	// Cancel client stream if active
	if w.clientStreamHandle != nil {
		w.clientStreamHandle = nil
		w.logger.Info("client stream cancelled by user")
		return
	}

	// If no operation is active, log it
	w.logger.Debug("no active operation to cancel")
}
