package browser

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/model"
	"github.com/shhac/grotto/internal/ui/settings"
)

// ConnectionBar represents the connection controls at the top of the browser panel
type ConnectionBar struct {
	widget.BaseWidget

	addressEntry *widget.Entry
	connectBtn   *widget.Button
	tlsBtn       *widget.Button
	state        *model.ConnectionUIState
	window       fyne.Window

	// TLS settings
	tlsSettings domain.TLSSettings

	onConnect    func(address string, tlsSettings domain.TLSSettings)
	onDisconnect func()

	container *fyne.Container
}

// NewConnectionBar creates a new connection bar widget
func NewConnectionBar(state *model.ConnectionUIState, window fyne.Window) *ConnectionBar {
	c := &ConnectionBar{
		state:  state,
		window: window,
	}

	c.addressEntry = widget.NewEntry()
	c.addressEntry.SetPlaceHolder("localhost:50051")

	c.connectBtn = widget.NewButton("Connect", func() {
		c.handleButtonClick()
	})

	// TLS settings button with icon
	c.tlsBtn = widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		c.showTLSSettings()
	})
	c.tlsBtn.Importance = widget.LowImportance

	// Layout: address entry, TLS button, connect button
	c.container = container.NewBorder(
		nil, nil,
		nil,
		container.NewHBox(c.tlsBtn, c.connectBtn),
		c.addressEntry,
	)

	// Listen to state changes to update the button
	state.State.AddListener(binding.NewDataListener(func() {
		c.updateButton()
	}))

	c.ExtendBaseWidget(c)
	return c
}

// SetOnConnect sets the callback for when the connect button is clicked while disconnected
func (c *ConnectionBar) SetOnConnect(fn func(address string, tlsSettings domain.TLSSettings)) {
	c.onConnect = fn
}

// SetOnDisconnect sets the callback for when the connect button is clicked while connected
func (c *ConnectionBar) SetOnDisconnect(fn func()) {
	c.onDisconnect = fn
}

// CreateRenderer creates the renderer for this widget
func (c *ConnectionBar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.container)
}

// handleButtonClick handles clicks on the connect/disconnect button
func (c *ConnectionBar) handleButtonClick() {
	state, err := c.state.State.Get()
	if err != nil {
		return
	}

	switch state {
	case "disconnected", "error":
		// Connect
		address := c.addressEntry.Text
		if address == "" {
			address = "localhost:50051" // Default
		}
		if c.onConnect != nil {
			c.onConnect(address, c.tlsSettings)
		}
	case "connected":
		// Disconnect
		if c.onDisconnect != nil {
			c.onDisconnect()
		}
	case "connecting":
		// Do nothing while connecting
	}
}

// showTLSSettings opens the TLS configuration dialog
func (c *ConnectionBar) showTLSSettings() {
	settings.ShowTLSDialog(c.window, c.tlsSettings, func(newSettings domain.TLSSettings) {
		c.tlsSettings = newSettings
	})
}

// updateButton updates the button text and state based on connection state
func (c *ConnectionBar) updateButton() {
	state, err := c.state.State.Get()
	if err != nil {
		return
	}

	switch state {
	case "disconnected":
		c.connectBtn.SetText("Connect")
		c.connectBtn.Enable()
		c.addressEntry.Enable()
	case "connecting":
		c.connectBtn.SetText("Connecting...")
		c.connectBtn.Disable()
		c.addressEntry.Disable()
	case "connected":
		c.connectBtn.SetText("Disconnect")
		c.connectBtn.Enable()
		c.addressEntry.Disable()
	case "error":
		c.connectBtn.SetText("Retry")
		c.connectBtn.Enable()
		c.addressEntry.Enable()
	}
}

// GetTLSSettings returns the current TLS settings
func (c *ConnectionBar) GetTLSSettings() domain.TLSSettings {
	return c.tlsSettings
}

// SetTLSSettings sets the TLS settings
func (c *ConnectionBar) SetTLSSettings(settings domain.TLSSettings) {
	c.tlsSettings = settings
}

// FocusAddress focuses the address entry field (for keyboard shortcut)
func (c *ConnectionBar) FocusAddress() {
	c.window.Canvas().Focus(c.addressEntry)
}

// SetAddress sets the address in the entry field
func (c *ConnectionBar) SetAddress(address string) {
	c.addressEntry.SetText(address)
}
