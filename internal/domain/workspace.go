package domain

// Workspace holds saved connections and requests
type Workspace struct {
	Name        string
	Connections []Connection
	Requests    []SavedRequest

	// Current UI state
	CurrentConnection *Connection // Active connection settings
	CurrentRequest    *Request    // Current request being edited
	SelectedService   string      // Currently selected service
	SelectedMethod    string      // Currently selected method
}

// SavedRequest represents a named request for reuse
type SavedRequest struct {
	Name    string
	Request Request
}
