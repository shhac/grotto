package errors

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/shhac/grotto/internal/model"
)

// StatusBar displays the current connection status with a shape-changing icon indicator.
// Each state uses a distinct icon shape for accessibility (not color-only):
//   - Disconnected: empty radio button (circle outline)
//   - Connecting: view-refresh icon (circular arrows)
//   - Connected: confirm icon (checkmark)
//   - Error: error icon (X shape)
type StatusBar struct {
	widget.BaseWidget

	state       *model.ConnectionUIState
	statusLabel *widget.Label
	indicator   *widget.Icon
}

// NewStatusBar creates a new status bar bound to the given connection state.
func NewStatusBar(state *model.ConnectionUIState) *StatusBar {
	label := widget.NewLabel("Disconnected")
	label.Truncation = fyne.TextTruncateEllipsis

	s := &StatusBar{
		state:       state,
		statusLabel: label,
		indicator:   widget.NewIcon(theme.RadioButtonIcon()),
	}
	s.ExtendBaseWidget(s)

	// Listen to state changes
	state.State.AddListener(binding.NewDataListener(s.updateStatus))
	state.Message.AddListener(binding.NewDataListener(s.updateStatus))

	// Set initial state
	s.updateStatus()

	return s
}

// updateStatus refreshes the status bar based on current state.
func (s *StatusBar) updateStatus() {
	stateStr, _ := s.state.State.Get()
	message, _ := s.state.Message.Get()

	switch stateStr {
	case "disconnected":
		s.indicator.SetResource(theme.RadioButtonIcon())
		if message == "" {
			s.statusLabel.SetText("Disconnected")
		} else {
			s.statusLabel.SetText(message)
		}

	case "connecting":
		s.indicator.SetResource(theme.ViewRefreshIcon())
		if message == "" {
			s.statusLabel.SetText("Connecting...")
		} else {
			s.statusLabel.SetText(message)
		}

	case "connected":
		s.indicator.SetResource(theme.ConfirmIcon())
		if message == "" {
			s.statusLabel.SetText("Connected")
		} else {
			s.statusLabel.SetText(message)
		}

	case "error":
		s.indicator.SetResource(theme.ErrorIcon())
		if message == "" {
			s.statusLabel.SetText("Connection Error")
		} else {
			s.statusLabel.SetText(message)
		}

	default:
		s.indicator.SetResource(theme.RadioButtonIcon())
		s.statusLabel.SetText("Unknown state")
	}

	s.statusLabel.Refresh()
}

// CreateRenderer implements fyne.Widget.
func (s *StatusBar) CreateRenderer() fyne.WidgetRenderer {
	// Create container with indicator icon and status label
	statusContainer := container.NewHBox(
		s.indicator,
		s.statusLabel,
	)

	return widget.NewSimpleRenderer(statusContainer)
}

// MinSize returns the minimum size for the status bar.
func (s *StatusBar) MinSize() fyne.Size {
	return s.BaseWidget.MinSize()
}

// SetState is a convenience method to update the connection state.
// State should be one of: "disconnected", "connecting", "connected", "error"
func (s *StatusBar) SetState(state string, message string) {
	_ = s.state.State.Set(state)
	_ = s.state.Message.Set(message)
}
