package errors

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	apperrors "github.com/shhac/grotto/internal/errors"
)

// ShowError displays a simple error dialog with the error message.
func ShowError(err error, window fyne.Window) {
	if err == nil {
		return
	}

	dialog.ShowError(err, window)
}

// ShowGRPCError displays a rich gRPC error dialog with recovery suggestions
// and technical details. The onRetry function is called when the user clicks
// the Retry button (if present).
func ShowGRPCError(err error, window fyne.Window, onRetry func()) {
	if err == nil {
		return
	}

	// Classify the error to get UI-friendly metadata
	uiErr := apperrors.ClassifyGRPCError(err)
	if uiErr == nil {
		// Fall back to simple error dialog
		dialog.ShowError(err, window)
		return
	}

	// Build dialog content with word-wrapping labels to prevent horizontal expansion
	msgLabel := widget.NewLabel(uiErr.Message)
	msgLabel.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(msgLabel)

	// Add recovery suggestions if available
	if len(uiErr.Recovery) > 0 {
		content.Add(widget.NewSeparator())
		content.Add(widget.NewLabel("You can:"))
		for _, suggestion := range uiErr.Recovery {
			lbl := widget.NewLabel("• " + suggestion)
			lbl.Wrapping = fyne.TextWrapWord
			content.Add(lbl)
		}
	}

	// Add expandable technical details if available
	if uiErr.Details != "" {
		detailsLabel := widget.NewLabel(uiErr.Details)
		detailsLabel.Wrapping = fyne.TextWrapWord
		accordion := widget.NewAccordion(
			widget.NewAccordionItem("Technical Details", detailsLabel),
		)
		content.Add(accordion)
	}

	// Check if there's a retry action and a handler
	hasRetry := false
	for _, action := range uiErr.Actions {
		if action.Label == "Retry" && onRetry != nil {
			hasRetry = true
			break
		}
	}

	// Create appropriate dialog type with explicit size to prevent window resizing
	if hasRetry {
		// Create dialog with retry button
		d := dialog.NewCustomConfirm(
			uiErr.Title,
			"Retry",
			"Close",
			content,
			func(retry bool) {
				if retry && onRetry != nil {
					onRetry()
				}
			},
			window,
		)
		d.Resize(fyne.NewSize(500, 400))
		d.Show()
	} else {
		// Create simple custom dialog
		d := dialog.NewCustom(uiErr.Title, "Close", content, window)
		d.Resize(fyne.NewSize(500, 400))
		d.Show()
	}
}
