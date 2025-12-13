package domain

// Workspace holds saved connections and requests
type Workspace struct {
	Name        string
	Connections []Connection
	Requests    []SavedRequest
}

// SavedRequest represents a named request for reuse
type SavedRequest struct {
	Name    string
	Request Request
}
