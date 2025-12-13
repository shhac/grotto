package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/grpc"
	"github.com/shhac/grotto/internal/model"
	"github.com/shhac/grotto/internal/ui/browser"
	uierrors "github.com/shhac/grotto/internal/ui/errors"
	"github.com/shhac/grotto/internal/ui/request"
	"github.com/shhac/grotto/internal/ui/response"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// AppController defines the interface for app-level operations needed by the UI
type AppController interface {
	State() *model.ApplicationState
	Logger() *slog.Logger
	InitializeReflectionClient() error
	CleanupReflectionClient()
	ConnManager() *grpc.ConnectionManager
	ReflectionClient() *grpc.ReflectionClient
	Invoker() *grpc.Invoker
}

// MainWindow manages the main application window and its layout.
type MainWindow struct {
	window fyne.Window
	state  *model.ApplicationState
	logger *slog.Logger
	app    AppController

	// Connection state for UI
	connState *model.ConnectionUIState

	// Panel widgets
	connectionBar *browser.ConnectionBar
	serviceBrowser *browser.ServiceBrowser
	requestPanel   *request.RequestPanel
	responsePanel  *response.ResponsePanel
	statusBar      *uierrors.StatusBar
}

// NewMainWindow creates a new main window with the application layout.
// The window is split horizontally with:
//   - Left side: Service Browser (service/method tree)
//   - Right side: Request Panel (top), Response Panel (middle), Status Bar (bottom)
func NewMainWindow(fyneApp fyne.App, app AppController) *MainWindow {
	// Create the window
	window := fyneApp.NewWindow("Grotto - gRPC Client")

	// Create connection state
	connState := model.NewConnectionUIState()

	mw := &MainWindow{
		window:    window,
		state:     app.State(),
		logger:    app.Logger(),
		app:       app,
		connState: connState,
	}

	// Create real UI components
	mw.connectionBar = browser.NewConnectionBar(connState)
	mw.serviceBrowser = browser.NewServiceBrowser(mw.state.Services)
	mw.requestPanel = request.NewRequestPanel(mw.state.Request, mw.logger)
	mw.responsePanel = response.NewResponsePanel(mw.state.Response)
	mw.statusBar = uierrors.NewStatusBar(connState)

	// Wire up callbacks
	mw.wireCallbacks()

	// Set up the window content
	mw.SetContent()

	// Set default window size
	window.Resize(fyne.NewSize(1200, 800))

	return mw
}

// wireCallbacks sets up all the event handlers and connects components
func (w *MainWindow) wireCallbacks() {
	// Connection flow
	w.connectionBar.SetOnConnect(func(address string) {
		w.handleConnect(address)
	})

	w.connectionBar.SetOnDisconnect(func() {
		w.handleDisconnect()
	})

	// Method selection
	w.serviceBrowser.SetOnMethodSelect(func(service domain.Service, method domain.Method) {
		w.handleMethodSelect(service, method)
	})

	// Send request
	w.requestPanel.SetOnSend(func(jsonStr string, metadata map[string]string) {
		w.handleSendRequest(jsonStr, metadata)
	})
}

// handleConnect establishes a connection and lists services
func (w *MainWindow) handleConnect(address string) {
	go func() {
		ctx := context.Background()

		// Update UI state
		_ = w.connState.State.Set("connecting")
		_ = w.connState.Message.Set("Connecting to " + address)

		// Connect
		cfg := domain.Connection{
			Address: address,
			UseTLS:  false, // TODO: Add TLS toggle in UI
		}

		if err := w.app.ConnManager().Connect(ctx, cfg); err != nil {
			w.logger.Error("connection failed", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to connect: " + err.Error())

			// Show error dialog
			dialog.ShowError(fmt.Errorf("connection failed: %w", err), w.window)
			return
		}

		// Initialize reflection client
		if err := w.app.InitializeReflectionClient(); err != nil {
			w.logger.Error("failed to initialize reflection", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to initialize reflection: " + err.Error())

			dialog.ShowError(fmt.Errorf("reflection failed: %w", err), w.window)
			return
		}

		// List services
		services, err := w.app.ReflectionClient().ListServices(ctx)
		if err != nil {
			w.logger.Error("failed to list services", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to list services: " + err.Error())

			dialog.ShowError(fmt.Errorf("failed to list services: %w", err), w.window)
			return
		}

		// Update state with services
		servicesInterface := make([]interface{}, len(services))
		for i, svc := range services {
			servicesInterface[i] = svc
		}
		_ = w.state.Services.Set(servicesInterface)

		// Update connection state
		_ = w.state.CurrentServer.Set(address)
		_ = w.state.Connected.Set(true)
		_ = w.connState.State.Set("connected")
		_ = w.connState.Message.Set("Connected to " + address)

		w.logger.Info("connection established and services loaded",
			slog.String("address", address),
			slog.Int("service_count", len(services)),
		)

		// Refresh the service browser
		w.serviceBrowser.Refresh()
	}()
}

// handleDisconnect closes the connection
func (w *MainWindow) handleDisconnect() {
	go func() {
		// Clean up reflection client
		w.app.CleanupReflectionClient()

		// Disconnect
		if err := w.app.ConnManager().Disconnect(); err != nil {
			w.logger.Error("disconnect failed", slog.Any("error", err))
			dialog.ShowError(err, w.window)
			return
		}

		// Clear UI state
		_ = w.state.Services.Set([]interface{}{})
		_ = w.state.Connected.Set(false)
		_ = w.state.CurrentServer.Set("")
		_ = w.state.SelectedService.Set("")
		_ = w.state.SelectedMethod.Set("")

		w.logger.Info("disconnected")
	}()
}

// handleMethodSelect updates the UI when a method is selected
func (w *MainWindow) handleMethodSelect(service domain.Service, method domain.Method) {
	w.logger.Debug("method selected",
		slog.String("service", service.FullName),
		slog.String("method", method.Name),
	)

	// Update state
	_ = w.state.SelectedService.Set(service.FullName)
	_ = w.state.SelectedMethod.Set(method.Name)

	// Get method descriptor
	refClient := w.app.ReflectionClient()
	if refClient == nil {
		w.logger.Warn("reflection client not initialized")
		// Update without descriptor (form will show placeholder)
		w.requestPanel.SetMethod(method.Name, nil)
		return
	}

	methodDesc, err := refClient.GetMethodDescriptor(service.FullName, method.Name)
	if err != nil {
		w.logger.Error("failed to get method descriptor", slog.Any("error", err))
		// Update without descriptor (form will show placeholder)
		w.requestPanel.SetMethod(method.Name, nil)
		return
	}

	// Convert to protoreflect descriptor
	inputType := methodDesc.GetInputType()
	var protoDesc protoreflect.MessageDescriptor
	if inputType != nil {
		// desc.MessageDescriptor implements protoreflect.MessageDescriptor
		protoDesc = inputType.UnwrapMessage()
	}

	// Update request panel with method descriptor
	w.requestPanel.SetMethod(method.Name, protoDesc)

	// Clear previous response
	_ = w.state.Response.TextData.Set("")
	_ = w.state.Response.Error.Set("")
	_ = w.state.Response.Duration.Set("")
}

// handleSendRequest invokes the selected RPC method
func (w *MainWindow) handleSendRequest(jsonStr string, metadataMap map[string]string) {
	go func() {
		ctx := context.Background()

		// Get selected method
		serviceName, _ := w.state.SelectedService.Get()
		methodName, _ := w.state.SelectedMethod.Get()

		if serviceName == "" || methodName == "" {
			dialog.ShowError(fmt.Errorf("no method selected"), w.window)
			return
		}

		w.logger.Debug("sending request",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)

		// Set loading state
		_ = w.state.Response.Loading.Set(true)
		_ = w.state.Response.Error.Set("")

		startTime := time.Now()

		// Get method descriptor
		refClient := w.app.ReflectionClient()
		if refClient == nil {
			_ = w.state.Response.Loading.Set(false)
			_ = w.state.Response.Error.Set("Reflection client not initialized")
			return
		}

		methodDesc, err := refClient.GetMethodDescriptor(serviceName, methodName)
		if err != nil {
			w.logger.Error("failed to get method descriptor", slog.Any("error", err))
			_ = w.state.Response.Loading.Set(false)
			_ = w.state.Response.Error.Set("Failed to get method descriptor: " + err.Error())
			return
		}

		// Convert metadata map to grpc metadata
		md := metadata.New(metadataMap)

		// Invoke RPC
		invoker := w.app.Invoker()
		if invoker == nil {
			_ = w.state.Response.Loading.Set(false)
			_ = w.state.Response.Error.Set("Invoker not initialized")
			return
		}

		respJSON, _, err := invoker.InvokeUnary(ctx, methodDesc, jsonStr, md)

		duration := time.Since(startTime)
		_ = w.state.Response.Loading.Set(false)

		if err != nil {
			w.logger.Error("RPC invocation failed", slog.Any("error", err))
			_ = w.state.Response.Error.Set(err.Error())
			return
		}

		// Pretty-print JSON response
		var prettyJSON interface{}
		if err := json.Unmarshal([]byte(respJSON), &prettyJSON); err == nil {
			prettyBytes, _ := json.MarshalIndent(prettyJSON, "", "  ")
			respJSON = string(prettyBytes)
		}

		// Update response
		_ = w.state.Response.TextData.Set(respJSON)
		_ = w.state.Response.Duration.Set(fmt.Sprintf("Duration: %v", duration.Round(time.Millisecond)))
		_ = w.state.Response.Error.Set("")

		w.logger.Info("RPC completed successfully",
			slog.String("method", methodName),
			slog.Duration("duration", duration),
		)
	}()
}

// SetContent builds and sets the main window layout.
// Layout structure:
//
//	┌─────────────────┬──────────────────────────────┐
//	│  Connection Bar │                              │
//	├─────────────────┼──────────────────────────────┤
//	│                 │      Request Panel           │
//	│  Service        ├──────────────────────────────┤
//	│  Browser        │      Response Panel          │
//	│                 ├──────────────────────────────┤
//	│                 │      Status Bar              │
//	└─────────────────┴──────────────────────────────┘
func (w *MainWindow) SetContent() {
	// Left side: connection bar + service browser
	leftPanel := container.NewBorder(
		w.connectionBar, // top (connection controls)
		nil,             // bottom
		nil,             // left
		nil,             // right
		w.serviceBrowser, // center (service tree)
	)

	// Right side: vertical split with request, response, and status
	rightPanel := container.NewBorder(
		nil,          // top
		w.statusBar,  // bottom (status bar)
		nil,          // left
		nil,          // right
		container.NewVSplit(
			w.requestPanel,  // top half
			w.responsePanel, // bottom half
		),
	)

	// Main layout: horizontal split with browser on left, panels on right
	mainSplit := container.NewHSplit(
		leftPanel,   // left side (connection + browser)
		rightPanel,  // right side (request/response/status)
	)

	// Set the initial split position (30% for browser, 70% for panels)
	mainSplit.SetOffset(0.3)

	w.window.SetContent(mainSplit)
}

// Window returns the underlying Fyne window.
func (w *MainWindow) Window() fyne.Window {
	return w.window
}
