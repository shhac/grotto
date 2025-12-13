package grpc

import (
	"context"
	"log/slog"
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

	// Configure keepalive parameters for GUI applications
	kaParams := keepalive.ClientParameters{
		Time:                10 * time.Second, // Ping every 10s
		Timeout:             3 * time.Second,  // Wait 3s for ping ack
		PermitWithoutStream: true,             // Keep alive even when idle
	}

	// Build dial options
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kaParams),
	}

	// Configure TLS/credentials
	var creds credentials.TransportCredentials
	if cfg.UseTLS {
		if cfg.Insecure {
			// TLS with insecure verification (skip cert validation)
			creds = credentials.NewTLS(nil)
			m.logger.Warn("using insecure TLS connection (skipping certificate verification)")
		} else {
			// Secure TLS with system cert pool
			creds = credentials.NewTLS(nil)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
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
		slog.Bool("tls", cfg.UseTLS),
	)
	m.updateState(StateConnected, "Connected to "+cfg.Address)

	return nil
}

// Disconnect closes the gRPC connection
func (m *ConnectionManager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		m.updateState(StateDisconnected, "Already disconnected")
		return nil
	}

	addr := m.address
	err := m.conn.Close()
	if err != nil {
		m.logger.Error("failed to close connection",
			slog.String("address", addr),
			slog.Any("error", err),
		)
		m.updateState(StateError, "Failed to disconnect: "+err.Error())
		return err
	}

	m.conn = nil
	m.address = ""
	m.logger.Info("gRPC connection closed", slog.String("address", addr))
	m.updateState(StateDisconnected, "Disconnected")

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

// updateState updates the connection state and invokes the callback if set
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
