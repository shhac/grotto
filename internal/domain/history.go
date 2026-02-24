package domain

import "time"

// HistoryEntry represents a record of a gRPC request/response for replay
type HistoryEntry struct {
	ID           string        `json:"id"`
	Timestamp    time.Time     `json:"timestamp"`
	Connection   Connection    `json:"connection"`
	Method       string        `json:"method"`                  // Full method name (e.g., "mypackage.MyService/MyMethod")
	Request      string        `json:"request"`                 // JSON request body
	Response     string        `json:"response"`                // JSON response body (for reference)
	Duration     time.Duration `json:"duration"`                // Request duration
	Status       string        `json:"status"`                  // "success" or "error"
	Error        string        `json:"error"`                   // Error message if failed
	Metadata     Metadata      `json:"metadata"`                // Request metadata/headers
	StreamType   string        `json:"stream_type,omitempty"`   // "unary", "server_stream", "client_stream", "bidi_stream"
	MessageCount int           `json:"message_count,omitempty"` // Number of messages for streaming RPCs
}

// Metadata represents request/response metadata
type Metadata struct {
	Request  map[string]string `json:"request"`  // Request headers
	Response map[string]string `json:"response"` // Response headers
}
