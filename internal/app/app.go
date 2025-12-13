package app

import (
	"fmt"
	"log/slog"

	"fyne.io/fyne/v2"
	"github.com/shhac/grotto/internal/grpc"
	"github.com/shhac/grotto/internal/logging"
	"github.com/shhac/grotto/internal/model"
	"github.com/shhac/grotto/internal/storage"
)

// App is the main application coordinator, responsible for wiring
// together all components and managing their lifecycle.
type App struct {
	fyneApp        fyne.App
	window         fyne.Window
	config         *Config
	logger         *slog.Logger
	connManager    *grpc.ConnectionManager
	storage        storage.Repository
	state          *model.ApplicationState
	reflectionClient *grpc.ReflectionClient
	invoker        *grpc.Invoker
}

// New creates a new App instance with the given configuration.
// This performs all dependency injection and wiring.
func New(fyneApp fyne.App, cfg *Config) (*App, error) {
	// Initialize logger
	logger, err := logging.InitLogger("grotto", cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	logger.Info("initializing Grotto application",
		slog.Bool("debug", cfg.Debug),
		slog.String("storage_path", cfg.StoragePath),
	)

	// Initialize storage
	storagePath := cfg.StoragePath
	if storagePath == "" {
		storagePath, err = storage.DefaultStoragePath()
		if err != nil {
			return nil, fmt.Errorf("failed to determine storage path: %w", err)
		}
	}

	repo := storage.NewJSONRepository(storagePath, logger)

	// Initialize connection manager
	connManager := grpc.NewConnectionManager(logger)

	// Initialize application state
	state := model.NewApplicationState()

	// Wire connection manager state changes to UI state
	connUIState := model.NewConnectionUIState()
	connManager.SetStateCallback(func(connState grpc.ConnectionState, message string) {
		// Map connection state to UI state string
		var uiState string
		switch connState {
		case grpc.StateDisconnected:
			uiState = "disconnected"
		case grpc.StateConnecting:
			uiState = "connecting"
		case grpc.StateConnected:
			uiState = "connected"
		case grpc.StateError:
			uiState = "error"
		default:
			uiState = "disconnected"
		}

		_ = connUIState.State.Set(uiState)
		_ = connUIState.Message.Set(message)

		// Also update the main state's Connected binding
		_ = state.Connected.Set(connState == grpc.StateConnected)
	})

	logger.Info("application initialized successfully")

	return &App{
		fyneApp:     fyneApp,
		config:      cfg,
		logger:      logger,
		connManager: connManager,
		storage:     repo,
		state:       state,
	}, nil
}

// Run starts the application and displays the main window.
// This is a blocking call that runs the Fyne event loop.
func (a *App) Run(window fyne.Window) {
	a.window = window
	a.logger.Info("starting application")
	a.window.ShowAndRun()
}

// ConnManager returns the connection manager for use by UI components.
func (a *App) ConnManager() *grpc.ConnectionManager {
	return a.connManager
}

// State returns the application state for use by UI components.
func (a *App) State() *model.ApplicationState {
	return a.state
}

// Logger returns the application logger.
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// Storage returns the storage repository.
func (a *App) Storage() storage.Repository {
	return a.storage
}

// FyneApp returns the underlying Fyne application instance.
func (a *App) FyneApp() fyne.App {
	return a.fyneApp
}

// ReflectionClient returns the reflection client (may be nil if not connected)
func (a *App) ReflectionClient() *grpc.ReflectionClient {
	return a.reflectionClient
}

// Invoker returns the RPC invoker (may be nil if not connected)
func (a *App) Invoker() *grpc.Invoker {
	return a.invoker
}

// InitializeReflectionClient creates a new reflection client and invoker for the current connection.
// This should be called after a successful connection is established.
func (a *App) InitializeReflectionClient() error {
	conn := a.connManager.Conn()
	if conn == nil {
		return fmt.Errorf("no active connection")
	}

	// Close old reflection client if it exists
	if a.reflectionClient != nil {
		a.reflectionClient.Close()
	}

	// Create new reflection client and invoker
	a.reflectionClient = grpc.NewReflectionClient(conn, a.logger)
	a.invoker = grpc.NewInvoker(conn, a.logger)

	a.logger.Info("reflection client and invoker initialized")
	return nil
}

// CleanupReflectionClient closes and clears the reflection client and invoker
func (a *App) CleanupReflectionClient() {
	if a.reflectionClient != nil {
		a.reflectionClient.Close()
		a.reflectionClient = nil
	}
	a.invoker = nil
	a.logger.Debug("reflection client and invoker cleaned up")
}
