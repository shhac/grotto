package domain

import "time"

// Connection holds gRPC connection settings
type Connection struct {
	Address string        `json:"Address"`
	UseTLS  bool          `json:"UseTLS"`
	Timeout time.Duration `json:"Timeout"`

	// TLS configuration
	TLS TLSSettings `json:"TLS"`
}

// TLSSettings holds detailed TLS configuration
type TLSSettings struct {
	Enabled        bool   `json:"Enabled"`
	SkipVerify     bool   `json:"SkipVerify"`     // Skip TLS certificate verification (insecure)
	CertFile       string `json:"CertFile"`       // Path to CA certificate
	ClientCertFile string `json:"ClientCertFile"` // Path to client certificate (mTLS)
	ClientKeyFile  string `json:"ClientKeyFile"`  // Path to client key (mTLS)
}
