package errors

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"

	"github.com/shhac/grotto/internal/model"
)

// Status indicator colors
var (
	colorDisconnected = color.RGBA{128, 128, 128, 255} // Gray
	colorConnecting   = color.RGBA{255, 193, 7, 255}   // Amber
	colorConnected    = color.RGBA{76, 175, 80, 255}   // Green
	colorError        = color.RGBA{244, 67, 54, 255}   // Red
)

// StatusBar displays the current connection status with a colored indicator dot.
// Format: "● Connected to api.example.com:50051"
type StatusBar struct {
	widget.BaseWidget

	state       *model.ConnectionUIState
	statusLabel *widget.Label
	indicator   *canvas.Circle
}

// NewStatusBar creates a new status bar bound to the given connection state.
func NewStatusBar(state *model.ConnectionUIState) *StatusBar {
	s := &StatusBar{
		state:       state,
		statusLabel: widget.NewLabel("Disconnected"),
		indicator:   canvas.NewCircle(colorDisconnected),
	}
	s.ExtendBaseWidget(s)

	// Set initial indicator size
	s.indicator.Resize(fyne.NewSize(12, 12))
	s.indicator.StrokeWidth = 0
	s.indicator.StrokeColor = color.Transparent

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
		s.indicator.FillColor = colorDisconnected
		if message == "" {
			s.statusLabel.SetText("Disconnected")
		} else {
			s.statusLabel.SetText(message)
		}

	case "connecting":
		s.indicator.FillColor = colorConnecting
		if message == "" {
			s.statusLabel.SetText("Connecting...")
		} else {
			s.statusLabel.SetText(message)
		}

	case "connected":
		s.indicator.FillColor = colorConnected
		if message == "" {
			s.statusLabel.SetText("Connected")
		} else {
			s.statusLabel.SetText(message)
		}

	case "error":
		s.indicator.FillColor = colorError
		if message == "" {
			s.statusLabel.SetText("Connection Error")
		} else {
			s.statusLabel.SetText(message)
		}

	default:
		s.indicator.FillColor = colorDisconnected
		s.statusLabel.SetText("Unknown state")
	}

	s.indicator.Refresh()
	s.statusLabel.Refresh()
}

// CreateRenderer implements fyne.Widget.
func (s *StatusBar) CreateRenderer() fyne.WidgetRenderer {
	// Create container with indicator dot and status label
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
