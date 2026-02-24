package model

import "fyne.io/fyne/v2/data/binding"

// ApplicationState represents the centralized application state with Fyne data bindings.
// All UI components bind to these values for reactive updates.
type ApplicationState struct {
	// Connection state
	CurrentServer binding.String
	Connected     binding.Bool

	// Selection state
	SelectedService binding.String
	SelectedMethod  binding.String

	// Request/Response state
	Request  *RequestState
	Response *ResponseState

	// Services discovered via reflection
	Services binding.UntypedList // []domain.Service
}

// NewApplicationState creates a new ApplicationState with initialized bindings.
func NewApplicationState() *ApplicationState {
	return &ApplicationState{
		CurrentServer:   binding.NewString(),
		Connected:       binding.NewBool(),
		SelectedService: binding.NewString(),
		SelectedMethod:  binding.NewString(),
		Request:         NewRequestState(),
		Response:        NewResponseState(),
		Services:        binding.NewUntypedList(),
	}
}

// RequestState represents the state of the request panel.
type RequestState struct {
	Mode     binding.String     // "text" or "form"
	TextData binding.String     // JSON representation
	Metadata binding.StringList // Request metadata headers
}

// NewRequestState creates a new RequestState with initialized bindings.
func NewRequestState() *RequestState {
	mode := binding.NewString()
	_ = mode.Set("form") // Default to form mode

	return &RequestState{
		Mode:     mode,
		TextData: binding.NewString(),
		Metadata: binding.NewStringList(),
	}
}

// ResponseState represents the state of the response panel.
type ResponseState struct {
	TextData binding.String // JSON response
	Loading  binding.Bool   // Whether request is in progress
	Error    binding.String // Error message if request failed
	Duration binding.String // Request duration (e.g., "123ms")
	Size     binding.String // Response body size (e.g., "1.2 KB")
}

// NewResponseState creates a new ResponseState with initialized bindings.
func NewResponseState() *ResponseState {
	loading := binding.NewBool()
	_ = loading.Set(false) // Default to not loading

	return &ResponseState{
		TextData: binding.NewString(),
		Loading:  loading,
		Error:    binding.NewString(),
		Duration: binding.NewString(),
		Size:     binding.NewString(),
	}
}

// ConnectionUIState represents the UI state for connection status display.
// States: "disconnected", "connecting", "connected", "error"
type ConnectionUIState struct {
	State   binding.String // Connection state
	Message binding.String // Status message
}

// NewConnectionUIState creates a new ConnectionUIState with initialized bindings.
func NewConnectionUIState() *ConnectionUIState {
	state := binding.NewString()
	_ = state.Set("disconnected") // Default to disconnected

	return &ConnectionUIState{
		State:   state,
		Message: binding.NewString(),
	}
}
