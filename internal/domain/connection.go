package domain

import "time"

// Connection holds gRPC connection settings
type Connection struct {
	Address string
	UseTLS  bool
	Timeout time.Duration

	// TLS configuration
	TLS TLSSettings
}

// TLSSettings holds detailed TLS configuration
type TLSSettings struct {
	Enabled        bool
	SkipVerify     bool   // Skip TLS certificate verification (insecure)
	CertFile       string // Path to CA certificate
	ClientCertFile string // Path to client certificate (mTLS)
	ClientKeyFile  string // Path to client key (mTLS)
}
