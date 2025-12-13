package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
)

// TLSConfig is a widget for configuring TLS connection settings
type TLSConfig struct {
	widget.BaseWidget

	// TLS settings
	enableTLS   *widget.Check
	skipVerify  *widget.Check
	certFile    *widget.Entry
	certFileBtn *widget.Button
	clientCert  *widget.Entry
	clientCertBtn *widget.Button
	clientKey   *widget.Entry
	clientKeyBtn *widget.Button

	// UI container
	container *fyne.Container
	window    fyne.Window // For file dialogs
}

// NewTLSConfig creates a new TLS configuration widget
func NewTLSConfig(window fyne.Window) *TLSConfig {
	t := &TLSConfig{
		window: window,
	}

	// Create widgets
	t.enableTLS = widget.NewCheck("Enable TLS", func(checked bool) {
		t.updateFieldStates()
	})

	t.skipVerify = widget.NewCheck("Skip certificate verification (insecure)", nil)

	// CA Certificate
	t.certFile = widget.NewEntry()
	t.certFile.SetPlaceHolder("Path to CA certificate (optional)")
	t.certFileBtn = widget.NewButton("Browse", func() {
		t.showFileDialog("Select CA Certificate", t.certFile)
	})

	// Client Certificate (mTLS)
	t.clientCert = widget.NewEntry()
	t.clientCert.SetPlaceHolder("Path to client certificate (optional)")
	t.clientCertBtn = widget.NewButton("Browse", func() {
		t.showFileDialog("Select Client Certificate", t.clientCert)
	})

	// Client Key (mTLS)
	t.clientKey = widget.NewEntry()
	t.clientKey.SetPlaceHolder("Path to client key (optional)")
	t.clientKeyBtn = widget.NewButton("Browse", func() {
		t.showFileDialog("Select Client Key", t.clientKey)
	})

	// Build layout
	t.buildLayout()

	// Initialize field states
	t.updateFieldStates()

	t.ExtendBaseWidget(t)
	return t
}

// buildLayout constructs the widget's UI layout
func (t *TLSConfig) buildLayout() {
	// CA Certificate row
	caCertRow := container.NewBorder(nil, nil, nil, t.certFileBtn, t.certFile)

	// Client Certificate row
	clientCertRow := container.NewBorder(nil, nil, nil, t.clientCertBtn, t.clientCert)

	// Client Key row
	clientKeyRow := container.NewBorder(nil, nil, nil, t.clientKeyBtn, t.clientKey)

	// Main container
	t.container = container.NewVBox(
		widget.NewLabel("TLS Configuration"),
		widget.NewSeparator(),
		t.enableTLS,
		t.skipVerify,
		widget.NewLabel("CA Certificate:"),
		caCertRow,
		widget.NewLabel("Client Certificate (mTLS):"),
		clientCertRow,
		widget.NewLabel("Client Key (mTLS):"),
		clientKeyRow,
	)
}

// showFileDialog opens a file picker dialog and sets the selected path to the entry
func (t *TLSConfig) showFileDialog(title string, entry *widget.Entry) {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, t.window)
			return
		}
		if reader == nil {
			return // User cancelled
		}
		defer reader.Close()

		// Set the file path in the entry
		entry.SetText(reader.URI().Path())
	}, t.window)

	fd.SetFilter(storage.NewExtensionFileFilter([]string{".pem", ".crt", ".key", ".cert"}))
	fd.SetFileName(title)
	fd.Show()
}

// updateFieldStates enables/disables fields based on the enable TLS checkbox
func (t *TLSConfig) updateFieldStates() {
	enabled := t.enableTLS.Checked

	if enabled {
		t.skipVerify.Enable()
		t.certFile.Enable()
		t.certFileBtn.Enable()
		t.clientCert.Enable()
		t.clientCertBtn.Enable()
		t.clientKey.Enable()
		t.clientKeyBtn.Enable()
	} else {
		t.skipVerify.Disable()
		t.certFile.Disable()
		t.certFileBtn.Disable()
		t.clientCert.Disable()
		t.clientCertBtn.Disable()
		t.clientKey.Disable()
		t.clientKeyBtn.Disable()
	}
}

// GetConfig returns the current TLS settings
func (t *TLSConfig) GetConfig() domain.TLSSettings {
	return domain.TLSSettings{
		Enabled:        t.enableTLS.Checked,
		SkipVerify:     t.skipVerify.Checked,
		CertFile:       t.certFile.Text,
		ClientCertFile: t.clientCert.Text,
		ClientKeyFile:  t.clientKey.Text,
	}
}

// SetConfig populates the widget from saved settings
func (t *TLSConfig) SetConfig(cfg domain.TLSSettings) {
	t.enableTLS.SetChecked(cfg.Enabled)
	t.skipVerify.SetChecked(cfg.SkipVerify)
	t.certFile.SetText(cfg.CertFile)
	t.clientCert.SetText(cfg.ClientCertFile)
	t.clientKey.SetText(cfg.ClientKeyFile)

	t.updateFieldStates()
}

// CreateRenderer implements the fyne.Widget interface
func (t *TLSConfig) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.container)
}
