package domain

// Workspace holds saved connections and requests
type Workspace struct {
	Name        string         `json:"Name"`
	Connections []Connection   `json:"Connections,omitempty"`
	Requests    []SavedRequest `json:"Requests,omitempty"`

	// Current UI state
	CurrentConnection *Connection `json:"CurrentConnection,omitempty"` // Active connection settings
	CurrentRequest    *Request    `json:"CurrentRequest,omitempty"`    // Current request being edited
	SelectedService   string      `json:"SelectedService"`             // Currently selected service
	SelectedMethod    string      `json:"SelectedMethod"`              // Currently selected method
}

// SavedRequest represents a named request for reuse
type SavedRequest struct {
	Name    string  `json:"Name"`
	Request Request `json:"Request"`
}
