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

// handleCancelOperation cancels any active streaming operation.
// Priority order: bidi > server stream > client stream > unary.
func (w *MainWindow) handleCancelOperation() {
	w.streamMu.Lock()
	bidiCancel := w.bidiCancelFunc
	serverCancel := w.serverStreamCancel
	clientHandle := w.clientStreamHandle
	unaryCancel := w.unaryCancel

	// Cancel in priority order
	switch {
	case bidiCancel != nil:
		w.bidiCancelFunc = nil
		w.bidiStreamHandle = nil
		w.streamMu.Unlock()
		bidiCancel()
		w.bidiPanel.SetStatus("Cancelled by user (Escape)")
		w.logger.Info("bidi stream cancelled by user")

	case serverCancel != nil:
		w.serverStreamCancel = nil
		w.streamMu.Unlock()
		serverCancel()
		w.logger.Info("server stream cancelled by user")

	case clientHandle != nil:
		w.clientStreamHandle = nil
		w.streamMu.Unlock()
		go clientHandle.CloseAndReceive()
		w.logger.Info("client stream cancelled by user")

	case unaryCancel != nil:
		w.unaryCancel = nil
		w.streamMu.Unlock()
		unaryCancel()
		w.logger.Info("unary request cancelled by user")

	default:
		w.streamMu.Unlock()
		w.logger.Debug("no active operation to cancel")
	}
}
