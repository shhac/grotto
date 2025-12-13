package response

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/model"
)

// ResponsePanel displays response data with reactive binding to state.
type ResponsePanel struct {
	widget.BaseWidget

	state         *model.ResponseState
	textDisplay   *widget.Entry // Read-only multiline for JSON
	errorLabel    *widget.Label
	durationLabel *widget.Label
	loadingBar    *widget.ProgressBarInfinite

	// Container for switching between content views
	contentContainer *fyne.Container
	responseContent  *fyne.Container
	errorContent     *fyne.Container
}

// NewResponsePanel creates a new response panel bound to the application state.
func NewResponsePanel(state *model.ResponseState) *ResponsePanel {
	p := &ResponsePanel{
		state: state,
	}
	p.ExtendBaseWidget(p)
	p.initializeComponents()
	p.setupBindings()
	return p
}

// initializeComponents creates all UI components.
func (p *ResponsePanel) initializeComponents() {
	// Response text display (read-only multiline entry)
	p.textDisplay = widget.NewMultiLineEntry()
	p.textDisplay.Wrapping = fyne.TextWrapWord
	p.textDisplay.Disable() // Read-only

	// Duration label
	p.durationLabel = widget.NewLabel("")

	// Loading bar (infinite progress)
	p.loadingBar = widget.NewProgressBarInfinite()
	p.loadingBar.Hide() // Hidden by default

	// Error label
	p.errorLabel = widget.NewLabel("")
	p.errorLabel.Wrapping = fyne.TextWrapWord

	// Create content containers
	p.responseContent = container.NewBorder(
		widget.NewLabel("Response:"),
		container.NewVBox(
			widget.NewSeparator(),
			p.durationLabel,
		),
		nil,
		nil,
		p.textDisplay,
	)

	p.errorContent = container.NewBorder(
		widget.NewLabel("Error:"),
		nil,
		nil,
		nil,
		p.errorLabel,
	)

	// Main content container (switches between response and error)
	p.contentContainer = container.NewStack(p.responseContent)
}

// setupBindings establishes reactive bindings to the state.
func (p *ResponsePanel) setupBindings() {
	// Bind text data to display
	p.textDisplay.Bind(p.state.TextData)

	// Bind duration
	p.durationLabel.Bind(p.state.Duration)

	// Listen to loading state
	p.state.Loading.AddListener(binding.NewDataListener(func() {
		loading, _ := p.state.Loading.Get()
		if loading {
			p.loadingBar.Start()
			p.loadingBar.Show()
		} else {
			p.loadingBar.Stop()
			p.loadingBar.Hide()
		}
	}))

	// Listen to error state
	p.state.Error.AddListener(binding.NewDataListener(func() {
		errorMsg, _ := p.state.Error.Get()
		if errorMsg != "" {
			p.errorLabel.SetText(errorMsg)
			p.showError()
		} else {
			p.showResponse()
		}
	}))
}

// showResponse displays the response content.
func (p *ResponsePanel) showResponse() {
	p.contentContainer.Objects = []fyne.CanvasObject{p.responseContent}
	p.contentContainer.Refresh()
}

// showError displays the error content.
func (p *ResponsePanel) showError() {
	p.contentContainer.Objects = []fyne.CanvasObject{p.errorContent}
	p.contentContainer.Refresh()
}

// SetResponse updates the panel with response data (convenience method).
func (p *ResponsePanel) SetResponse(json string, duration string) {
	_ = p.state.TextData.Set(json)
	_ = p.state.Duration.Set("Duration: " + duration)
	_ = p.state.Error.Set("") // Clear any previous error
}

// SetError shows an error message (convenience method).
func (p *ResponsePanel) SetError(message string) {
	_ = p.state.Error.Set(message)
	_ = p.state.TextData.Set("") // Clear response data
	_ = p.state.Duration.Set("")
}

// SetLoading shows/hides loading indicator (convenience method).
func (p *ResponsePanel) SetLoading(loading bool) {
	_ = p.state.Loading.Set(loading)
}

// CreateRenderer implements fyne.Widget.
func (p *ResponsePanel) CreateRenderer() fyne.WidgetRenderer {
	// Main layout with loading bar at bottom
	content := container.NewBorder(
		nil,
		p.loadingBar,
		nil,
		nil,
		p.contentContainer,
	)

	return widget.NewSimpleRenderer(content)
}

// MinSize implements fyne.Widget (optional, provides reasonable defaults).
func (p *ResponsePanel) MinSize() fyne.Size {
	return fyne.NewSize(400, 300)
}
