package domain

import "time"

// Request represents a gRPC request
type Request struct {
	Method   string            `json:"Method"`
	Body     string            `json:"Body"` // JSON
	Metadata map[string]string `json:"Metadata"`
}

// Response represents a gRPC response
type Response struct {
	Body     string            `json:"Body"` // JSON
	Metadata map[string]string `json:"Metadata"`
	Error    error             `json:"Error"`
	Duration time.Duration     `json:"Duration"`
}
