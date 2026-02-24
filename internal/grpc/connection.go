package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/shhac/grotto/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ConnectionState represents the current state of the gRPC connection
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateError
)

// String returns a human-readable representation of the connection state
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ConnectionManager manages the lifecycle of a gRPC client connection
type ConnectionManager struct {
	conn    *grpc.ClientConn
	state   ConnectionState
	address string
	logger  *slog.Logger
	mu      sync.RWMutex

	// Callbacks for state changes
	onStateChange func(state ConnectionState, message string)
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(logger *slog.Logger) *ConnectionManager {
	return &ConnectionManager{
		state:  StateDisconnected,
		logger: logger,
	}
}

// Connect establishes a gRPC connection with the provided configuration
func (m *ConnectionManager) Connect(ctx context.Context, cfg domain.Connection) error {
	m.updateState(StateConnecting, "Connecting to "+cfg.Address)

	// Configure keepalive parameters to avoid ENHANCE_YOUR_CALM errors
	kaParams := keepalive.ClientParameters{
		Time:                30 * time.Second, // Ping every 30s (reduced frequency)
		Timeout:             20 * time.Second, // Wait 20s for ping ack
		PermitWithoutStream: true,             // Keep alive even when idle
	}

	// Build dial options
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kaParams),
	}

	// Configure TLS/credentials
	var creds credentials.TransportCredentials
	if cfg.TLS.Enabled {
		// Build TLS configuration
		tlsConfig, err := m.buildTLSConfig(cfg.TLS)
		if err != nil {
			m.logger.Error("failed to build TLS config",
				slog.String("address", cfg.Address),
				slog.Any("error", err),
			)
			m.updateState(StateError, "Failed to configure TLS: "+err.Error())
			return err
		}

		creds = credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))

		if cfg.TLS.SkipVerify {
			m.logger.Warn("using insecure TLS connection (skipping certificate verification)")
		}
	} else {
		// No TLS (insecure plaintext)
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		m.logger.Warn("using insecure plaintext connection")
	}

	// Set timeout if configured
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	// Create the connection (not deprecated NewClient, not Dial)
	conn, err := grpc.NewClient(cfg.Address, opts...)
	if err != nil {
		m.logger.Error("failed to create gRPC client",
			slog.String("address", cfg.Address),
			slog.Any("error", err),
		)
		m.updateState(StateError, "Failed to connect: "+err.Error())
		return err
	}

	// Update state with new connection
	m.mu.Lock()
	// Close old connection if it exists
	if m.conn != nil {
		oldConn := m.conn
		go func() {
			if err := oldConn.Close(); err != nil {
				m.logger.Warn("failed to close old connection", slog.Any("error", err))
			}
		}()
	}
	m.conn = conn
	m.address = cfg.Address
	m.mu.Unlock()

	m.logger.Info("gRPC connection established",
		slog.String("address", cfg.Address),
		slog.Bool("tls", cfg.TLS.Enabled),
	)
	m.updateState(StateConnected, "Connected to "+cfg.Address)

	return nil
}

// Disconnect closes the gRPC connection
func (m *ConnectionManager) Disconnect() error {
	m.mu.Lock()

	if m.conn == nil {
		cb := m.updateStateLocked(StateDisconnected, "Already disconnected")
		m.mu.Unlock()
		if cb != nil {
			cb(StateDisconnected, "Already disconnected")
		}
		return nil
	}

	addr := m.address
	err := m.conn.Close()
	if err != nil {
		m.logger.Error("failed to close connection",
			slog.String("address", addr),
			slog.Any("error", err),
		)
		msg := "Failed to disconnect: " + err.Error()
		cb := m.updateStateLocked(StateError, msg)
		m.mu.Unlock()
		if cb != nil {
			cb(StateError, msg)
		}
		return err
	}

	m.conn = nil
	m.address = ""
	m.logger.Info("gRPC connection closed", slog.String("address", addr))
	cb := m.updateStateLocked(StateDisconnected, "Disconnected")
	m.mu.Unlock()
	if cb != nil {
		cb(StateDisconnected, "Disconnected")
	}

	return nil
}

// Conn returns the current gRPC client connection
// Returns nil if not connected
func (m *ConnectionManager) Conn() *grpc.ClientConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conn
}

// State returns the current connection state
func (m *ConnectionManager) State() ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Address returns the current connection address
func (m *ConnectionManager) Address() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.address
}

// SetStateCallback registers a callback function to be called on state changes
func (m *ConnectionManager) SetStateCallback(fn func(state ConnectionState, message string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStateChange = fn
}

// updateState updates the connection state and invokes the callback if set.
// The caller must NOT hold m.mu.
func (m *ConnectionManager) updateState(state ConnectionState, message string) {
	m.mu.Lock()
	m.state = state
	callback := m.onStateChange
	m.mu.Unlock()

	m.logger.Debug("connection state changed",
		slog.String("state", state.String()),
		slog.String("message", message),
	)

	if callback != nil {
		callback(state, message)
	}
}

// updateStateLocked updates the connection state while m.mu is already held.
// Returns the callback (if any) for the caller to invoke AFTER releasing the lock.
func (m *ConnectionManager) updateStateLocked(state ConnectionState, message string) func(ConnectionState, string) {
	m.state = state
	m.logger.Debug("connection state changed",
		slog.String("state", state.String()),
		slog.String("message", message),
	)
	return m.onStateChange
}

// buildTLSConfig creates a TLS configuration from TLSSettings
func (m *ConnectionManager) buildTLSConfig(settings domain.TLSSettings) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: settings.SkipVerify,
	}

	// Load CA certificate if provided
	if settings.CertFile != "" {
		caCert, err := os.ReadFile(settings.CertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		tlsConfig.RootCAs = caCertPool

		m.logger.Debug("loaded CA certificate", slog.String("file", settings.CertFile))
	}

	// Load client certificate and key for mTLS if provided
	if settings.ClientCertFile != "" && settings.ClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(settings.ClientCertFile, settings.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}

		m.logger.Debug("loaded client certificate for mTLS",
			slog.String("cert", settings.ClientCertFile),
			slog.String("key", settings.ClientKeyFile),
		)
	} else if settings.ClientCertFile != "" || settings.ClientKeyFile != "" {
		// Only one of cert/key provided - error
		return nil, fmt.Errorf("both client certificate and key must be provided for mTLS")
	}

	return tlsConfig, nil
}
