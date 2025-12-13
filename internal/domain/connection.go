package domain

import "time"

// Connection holds gRPC connection settings
type Connection struct {
	Address  string
	UseTLS   bool
	Insecure bool          // Skip TLS verification
	Timeout  time.Duration
}
