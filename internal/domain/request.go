package domain

import "time"

// Request represents a gRPC request
type Request struct {
	Method   string
	Body     string // JSON
	Metadata map[string]string
}

// Response represents a gRPC response
type Response struct {
	Body     string // JSON
	Metadata map[string]string
	Error    error
	Duration time.Duration
}
