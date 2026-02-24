package browser

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/model"
	"github.com/shhac/grotto/internal/storage"
	"github.com/shhac/grotto/internal/ui/settings"
)

// ConnectionBar represents the connection controls at the top of the browser panel
type ConnectionBar struct {
	widget.BaseWidget

	addressEntry *widget.SelectEntry
	connectBtn   *widget.Button
	tlsBtn       *widget.Button
	tlsToggleBtn *widget.Button
	state        *model.ConnectionUIState
	window       fyne.Window
	storage      storage.Repository
	recentConns  []domain.Connection

	// TLS settings
	tlsSettings domain.TLSSettings

	onConnect    func(address string, tlsSettings domain.TLSSettings)
	onDisconnect func()

	container *fyne.Container
}

// NewConnectionBar creates a new connection bar widget
func NewConnectionBar(state *model.ConnectionUIState, window fyne.Window, repo storage.Repository) *ConnectionBar {
	c := &ConnectionBar{
		state:   state,
		window:  window,
		storage: repo,
	}

	c.addressEntry = widget.NewSelectEntry(nil)
	c.addressEntry.SetPlaceHolder("localhost:50051")
	c.addressEntry.OnSubmitted = func(s string) {
		c.handleButtonClick()
	}
	c.loadRecentOptions()

	c.connectBtn = widget.NewButton("Connect", func() {
		c.handleButtonClick()
	})

	// TLS toggle button with padlock icon (left of address)
	c.tlsToggleBtn = widget.NewButtonWithIcon("", lockUnlockedIcon, func() {
		c.tlsSettings.Enabled = !c.tlsSettings.Enabled
		c.updateTLSIcon()
	})
	c.tlsToggleBtn.Importance = widget.LowImportance

	// TLS settings button with gear icon (advanced settings)
	c.tlsBtn = widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		c.showTLSSettings()
	})
	c.tlsBtn.Importance = widget.LowImportance

	// Layout: [padlock] [address entry] [gear] [connect]
	c.container = container.NewBorder(
		nil, nil,
		c.tlsToggleBtn,
		container.NewHBox(c.tlsBtn, c.connectBtn),
		c.addressEntry,
	)

	// Listen to state changes to update the button
	state.State.AddListener(binding.NewDataListener(func() {
		c.updateButton()
	}))

	// Initialize button state based on current connection state
	c.updateButton()

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
		address := c.resolveAddress()
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
		c.updateTLSIcon()
	})
}

// updateTLSIcon syncs the padlock icon with the current TLS enabled state.
func (c *ConnectionBar) updateTLSIcon() {
	if c.tlsSettings.Enabled {
		c.tlsToggleBtn.SetIcon(lockLockedIcon)
	} else {
		c.tlsToggleBtn.SetIcon(lockUnlockedIcon)
	}
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
		c.addressEntry.OnChanged = c.restoreTLSFromHistory
		c.addressEntry.Enable()
		c.tlsToggleBtn.Enable()
	case "connecting":
		c.connectBtn.SetText("Connecting...")
		c.connectBtn.Disable()
		c.addressEntry.OnChanged = nil
		c.addressEntry.Disable()
		c.tlsToggleBtn.Disable()
	case "connected":
		c.connectBtn.SetText("Disconnect")
		c.connectBtn.Enable()
		// Keep entry enabled for readable text contrast; prevent edits via OnChanged guard.
		connectedAddr := c.addressEntry.Text
		c.addressEntry.OnChanged = func(s string) {
			if s != connectedAddr {
				c.addressEntry.SetText(connectedAddr)
			}
		}
		c.tlsToggleBtn.Disable()
	case "error":
		c.connectBtn.SetText("Retry")
		c.connectBtn.Enable()
		c.addressEntry.OnChanged = c.restoreTLSFromHistory
		c.addressEntry.Enable()
		c.tlsToggleBtn.Enable()
	}
}

// GetTLSSettings returns the current TLS settings
func (c *ConnectionBar) GetTLSSettings() domain.TLSSettings {
	return c.tlsSettings
}

// SetTLSSettings sets the TLS settings and updates the padlock icon.
func (c *ConnectionBar) SetTLSSettings(s domain.TLSSettings) {
	c.tlsSettings = s
	c.updateTLSIcon()
}

// FocusAddress focuses the address entry field (for keyboard shortcut)
func (c *ConnectionBar) FocusAddress() {
	c.window.Canvas().Focus(c.addressEntry)
}

// TriggerConnect programmatically triggers the connect/disconnect action (for keyboard shortcut).
func (c *ConnectionBar) TriggerConnect() {
	c.handleButtonClick()
}

// SetAddress sets the address in the entry field
func (c *ConnectionBar) SetAddress(address string) {
	c.addressEntry.SetText(address)
}

// SaveConnection persists the given connection to recent connections and refreshes the dropdown.
func (c *ConnectionBar) SaveConnection(conn domain.Connection) {
	if err := c.storage.SaveRecentConnection(conn); err != nil {
		return
	}
	fyne.Do(func() {
		c.loadRecentOptions()
	})
}

// loadRecentOptions populates the address dropdown from stored recent connections.
func (c *ConnectionBar) loadRecentOptions() {
	conns, err := c.storage.GetRecentConnections()
	if err != nil || len(conns) == 0 {
		return
	}
	c.recentConns = conns
	options := make([]string, len(conns))
	for i, conn := range conns {
		options[i] = formatConnectionDisplay(conn)
	}
	c.addressEntry.SetOptions(options)
}

// formatConnectionDisplay returns a display string for a connection.
// If the connection has a name, formats as "Name (address)", otherwise just the address.
func formatConnectionDisplay(conn domain.Connection) string {
	if conn.Name != "" {
		return conn.Name + " (" + conn.Address + ")"
	}
	return conn.Address
}

// restoreTLSFromHistory restores TLS settings when an address matches a recent connection.
func (c *ConnectionBar) restoreTLSFromHistory(addr string) {
	for _, conn := range c.recentConns {
		if conn.Address == addr || formatConnectionDisplay(conn) == addr {
			c.tlsSettings = conn.TLS
			c.updateTLSIcon()
			return
		}
	}
}

// resolveAddress extracts the raw address from the entry text.
// Handles both plain addresses and "Name (address)" format from named profiles.
func (c *ConnectionBar) resolveAddress() string {
	text := c.addressEntry.Text
	// Check if it matches a named profile display format
	for _, conn := range c.recentConns {
		if formatConnectionDisplay(conn) == text {
			return conn.Address
		}
	}
	return text
}
