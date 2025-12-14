package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/jhump/protoreflect/desc"
	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/grpc"
	"github.com/shhac/grotto/internal/model"
	"github.com/shhac/grotto/internal/storage"
	"github.com/shhac/grotto/internal/ui/bidi"
	"github.com/shhac/grotto/internal/ui/browser"
	uierrors "github.com/shhac/grotto/internal/ui/errors"
	"github.com/shhac/grotto/internal/ui/history"
	"github.com/shhac/grotto/internal/ui/request"
	"github.com/shhac/grotto/internal/ui/response"
	"github.com/shhac/grotto/internal/ui/workspace"
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
	Storage() storage.Repository
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
	connectionBar  *browser.ConnectionBar
	serviceBrowser *browser.ServiceBrowser
	requestPanel   *request.RequestPanel
	responsePanel  *response.ResponsePanel
	bidiPanel      *bidi.BidiStreamPanel
	statusBar      *uierrors.StatusBar
	workspacePanel *workspace.WorkspacePanel
	historyPanel   *history.HistoryPanel
	themeSelector  *widget.Select

	// Streaming state
	clientStreamHandle *grpc.ClientStreamHandle
	bidiStreamHandle   *grpc.BidiStreamHandle
	bidiCancelFunc     context.CancelFunc
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
	mw.connectionBar = browser.NewConnectionBar(connState, window)
	mw.serviceBrowser = browser.NewServiceBrowser(mw.state.Services)
	mw.requestPanel = request.NewRequestPanel(mw.state.Request, mw.logger)
	mw.responsePanel = response.NewResponsePanel(mw.state.Response)
	mw.bidiPanel = bidi.NewBidiStreamPanel()
	mw.statusBar = uierrors.NewStatusBar(connState)
	mw.workspacePanel = workspace.NewWorkspacePanel(app.Storage(), app.Logger(), window)
	mw.historyPanel = history.NewHistoryPanel(app.Storage(), app.Logger())
	mw.themeSelector = CreateThemeSelector(fyneApp)

	// Wire up callbacks
	mw.wireCallbacks()

	// Set up the window content
	mw.SetContent()

	// Set up the main menu
	mw.setupMainMenu()

	// Set up keyboard shortcuts
	mw.setupKeyboardShortcuts()

	// Set default window size
	window.Resize(fyne.NewSize(1200, 800))

	return mw
}

// wireCallbacks sets up all the event handlers and connects components
func (w *MainWindow) wireCallbacks() {
	// Connection flow
	w.connectionBar.SetOnConnect(func(address string, tlsSettings domain.TLSSettings) {
		w.handleConnect(address, tlsSettings)
	})

	w.connectionBar.SetOnDisconnect(func() {
		w.handleDisconnect()
	})

	// Method selection
	w.serviceBrowser.SetOnMethodSelect(func(service domain.Service, method domain.Method) {
		w.handleMethodSelect(service, method)
	})

	// Send request (unary/server streaming)
	w.requestPanel.SetOnSend(func(jsonStr string, metadata map[string]string) {
		w.handleSendRequest(jsonStr, metadata)
	})

	// Client streaming: send message
	w.requestPanel.SetOnStreamSend(func(jsonStr string, metadata map[string]string) {
		w.handleClientStreamSend(jsonStr, metadata)
	})

	// Client streaming: finish and get response
	w.requestPanel.SetOnStreamEnd(func(metadata map[string]string) {
		w.handleClientStreamFinish(metadata)
	})

	// Workspace operations
	w.workspacePanel.SetOnSave(func() domain.Workspace {
		return w.captureWorkspaceState()
	})

	w.workspacePanel.SetOnLoad(func(workspace domain.Workspace) {
		w.applyWorkspaceState(workspace)
	})

	// History replay
	// TODO: Implement handleHistoryReplay method
	// w.historyPanel.SetOnReplay(func(entry domain.HistoryEntry) {
	// 	w.handleHistoryReplay(entry)
	// })
}

// handleConnect establishes a connection and lists services
func (w *MainWindow) handleConnect(address string, tlsSettings domain.TLSSettings) {
	go func() {
		ctx := context.Background()

		// Update UI state
		_ = w.connState.State.Set("connecting")
		_ = w.connState.Message.Set("Connecting to " + address)

		// Connect
		cfg := domain.Connection{
			Address: address,
			UseTLS:  tlsSettings.Enabled, // Use TLS if enabled in settings
			TLS:     tlsSettings,
		}

		if err := w.app.ConnManager().Connect(ctx, cfg); err != nil {
			w.logger.Error("connection failed", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to connect: " + err.Error())

			// Show rich gRPC error dialog with retry option
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - attempt connection again
				w.handleConnect(address, tlsSettings)
			})
			return
		}

		// Initialize reflection client
		if err := w.app.InitializeReflectionClient(); err != nil {
			w.logger.Error("failed to initialize reflection", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to initialize reflection: " + err.Error())

			// Show rich gRPC error dialog with retry option
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - attempt connection again
				w.handleConnect(address, tlsSettings)
			})
			return
		}

		// List services
		services, err := w.app.ReflectionClient().ListServices(ctx)
		if err != nil {
			w.logger.Error("failed to list services", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to list services: " + err.Error())

			// Show rich gRPC error dialog with retry option
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - attempt connection again
				w.handleConnect(address, tlsSettings)
			})
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

		// Update connection state to reflect disconnection
		_ = w.connState.State.Set("disconnected")
		_ = w.connState.Message.Set("Disconnected")

		// Refresh the service browser to clear the tree
		w.serviceBrowser.Refresh()

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

	// Check if this is a bidirectional streaming method
	isBidiStreaming := method.IsClientStream && method.IsServerStream

	if isBidiStreaming {
		// For bidi streaming, switch to bidi panel and set up callbacks
		w.switchToBidiPanel()
		w.bidiPanel.Clear()
		w.bidiPanel.SetOnSend(func(json string) {
			w.handleBidiStreamSend(json, make(map[string]string))
		})
		w.bidiPanel.SetOnCloseSend(func() {
			w.handleBidiStreamClose()
		})
		w.bidiPanel.SetStatus("Ready to start bidirectional stream")
	} else {
		// For other method types, use normal request/response panels
		w.switchToNormalPanel()

		// Update request panel with method descriptor
		w.requestPanel.SetMethod(method.Name, protoDesc)

		// Set client streaming mode based on method type
		w.requestPanel.SetClientStreaming(method.IsClientStream)

		// Clear previous response
		_ = w.state.Response.TextData.Set("")
		_ = w.state.Response.Error.Set("")
		_ = w.state.Response.Duration.Set("")
		w.responsePanel.ClearResponseMetadata()
	}

	// Log method type for debugging
	w.logger.Debug("method type detected",
		slog.String("method_type", method.MethodType()),
		slog.Bool("is_client_stream", method.IsClientStream),
		slog.Bool("is_server_stream", method.IsServerStream),
		slog.Bool("is_bidi_stream", isBidiStreaming),
	)
}

// handleSendRequest invokes the selected RPC method
func (w *MainWindow) handleSendRequest(jsonStr string, metadataMap map[string]string) {
	// Get selected method
	serviceName, _ := w.state.SelectedService.Get()
	methodName, _ := w.state.SelectedMethod.Get()

	if serviceName == "" || methodName == "" {
		dialog.ShowError(fmt.Errorf("no method selected"), w.window)
		return
	}

	// Get method descriptor
	refClient := w.app.ReflectionClient()
	if refClient == nil {
		_ = w.state.Response.Error.Set("Reflection client not initialized")
		return
	}

	methodDesc, err := refClient.GetMethodDescriptor(serviceName, methodName)
	if err != nil {
		w.logger.Error("failed to get method descriptor", slog.Any("error", err))
		_ = w.state.Response.Error.Set("Failed to get method descriptor: " + err.Error())
		return
	}

	// Check if this is a server streaming RPC
	if methodDesc.IsServerStreaming() {
		w.handleServerStreamRequest(jsonStr, metadataMap, methodDesc)
	} else {
		w.handleUnaryRequest(jsonStr, metadataMap, methodDesc)
	}
}

// handleUnaryRequest handles unary RPC invocations
func (w *MainWindow) handleUnaryRequest(jsonStr string, metadataMap map[string]string, methodDesc *desc.MethodDescriptor) {
	go func() {
		ctx := context.Background()

		serviceName, _ := w.state.SelectedService.Get()
		methodName, _ := w.state.SelectedMethod.Get()

		w.logger.Debug("sending unary request",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)

		// Set loading state and switch to normal response mode
		_ = w.state.Response.Loading.Set(true)
		_ = w.state.Response.Error.Set("")
		w.responsePanel.SetStreaming(false)

		startTime := time.Now()

		// Convert metadata map to grpc metadata
		md := metadata.New(metadataMap)

		// Invoke RPC
		invoker := w.app.Invoker()
		if invoker == nil {
			_ = w.state.Response.Loading.Set(false)
			_ = w.state.Response.Error.Set("Invoker not initialized")
			return
		}

		respJSON, respHeaders, err := invoker.InvokeUnary(ctx, methodDesc, jsonStr, md)

		duration := time.Since(startTime)
		_ = w.state.Response.Loading.Set(false)

		// Record history entry
		// TODO: Implement recordHistoryEntry method
		// currentServer, _ := w.state.CurrentServer.Get()
		// w.recordHistoryEntry(currentServer, serviceName+"/"+methodName, jsonStr, metadataMap, respJSON, respHeaders, duration, err)

		if err != nil {
			w.logger.Error("RPC invocation failed", slog.Any("error", err))

			// Show rich gRPC error dialog with retry option
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - send the request again
				w.handleSendRequest(jsonStr, metadataMap)
			})

			// Also set error in response panel for inline visibility
			_ = w.state.Response.Error.Set(err.Error())
			w.responsePanel.ClearResponseMetadata()
			return
		}

		// Pretty-print JSON response
		var prettyJSON interface{}
		if err := json.Unmarshal([]byte(respJSON), &prettyJSON); err == nil {
			prettyBytes, _ := json.MarshalIndent(prettyJSON, "", "  ")
			respJSON = string(prettyBytes)
		}

		// Convert metadata.MD to map[string]string for display
		metadataMap := make(map[string]string)
		for key, values := range respHeaders {
			if len(values) > 0 {
				// Join multiple values with comma
				metadataMap[key] = values[0]
				if len(values) > 1 {
					for i := 1; i < len(values); i++ {
						metadataMap[key] += ", " + values[i]
					}
				}
			}
		}

		// Update response
		_ = w.state.Response.TextData.Set(respJSON)
		_ = w.state.Response.Duration.Set(fmt.Sprintf("Duration: %v", duration.Round(time.Millisecond)))
		_ = w.state.Response.Error.Set("")
		w.responsePanel.SetResponseMetadata(metadataMap)

		w.logger.Info("RPC completed successfully",
			slog.String("method", methodName),
			slog.Duration("duration", duration),
		)
	}()
}

// handleServerStreamRequest handles server streaming RPC invocations
func (w *MainWindow) handleServerStreamRequest(jsonStr string, metadataMap map[string]string, methodDesc *desc.MethodDescriptor) {
	ctx, cancel := context.WithCancel(context.Background())

	serviceName, _ := w.state.SelectedService.Get()
	methodName, _ := w.state.SelectedMethod.Get()

	w.logger.Debug("sending server stream request",
		slog.String("service", serviceName),
		slog.String("method", methodName),
	)

	// Switch to streaming mode and prepare UI
	w.responsePanel.SetStreaming(true)
	streamWidget := w.responsePanel.StreamingWidget()
	streamWidget.Clear()
	streamWidget.SetStatus("Starting stream...")
	streamWidget.EnableStopButton()

	// Set stop button handler
	streamWidget.SetOnStop(func() {
		w.logger.Info("user requested stream stop")
		cancel()
		streamWidget.DisableStopButton()
		streamWidget.SetStatus("Stopped by user")
	})

	// Convert metadata map to grpc metadata
	md := metadata.New(metadataMap)

	// Invoke server streaming RPC
	invoker := w.app.Invoker()
	if invoker == nil {
		streamWidget.SetStatus("Error: Invoker not initialized")
		streamWidget.DisableStopButton()
		return
	}

	startTime := time.Now()
	msgChan, errChan := invoker.InvokeServerStream(ctx, methodDesc, jsonStr, md)

	// Process messages in a goroutine
	go func() {
		messageCount := 0

		for {
			select {
			case jsonMsg, ok := <-msgChan:
				if !ok {
					// Channel closed
					return
				}

				messageCount++

				// Pretty-print JSON message
				var prettyJSON interface{}
				if err := json.Unmarshal([]byte(jsonMsg), &prettyJSON); err == nil {
					prettyBytes, _ := json.MarshalIndent(prettyJSON, "", "  ")
					jsonMsg = string(prettyBytes)
				}

				// Add message to UI (bindings are thread-safe)
				streamWidget.AddMessage(jsonMsg)

			case err, ok := <-errChan:
				if !ok {
					// Channel closed
					return
				}

				duration := time.Since(startTime)

				// Check if this is normal stream completion (io.EOF) or an error
				if err == io.EOF {
					w.logger.Info("server stream completed successfully",
						slog.String("method", methodName),
						slog.Int("message_count", messageCount),
						slog.Duration("duration", duration),
					)

					streamWidget.SetStatus(fmt.Sprintf("Complete (%d messages in %v)", messageCount, duration.Round(time.Millisecond)))
					streamWidget.DisableStopButton()
				} else {
					w.logger.Error("server stream error",
						slog.String("method", methodName),
						slog.Int("message_count", messageCount),
						slog.Any("error", err),
					)

					streamWidget.SetStatus(fmt.Sprintf("Error: %s (received %d messages)", err.Error(), messageCount))
					streamWidget.DisableStopButton()
				}

				return
			}
		}
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
//	├─────────────────┼──────────────────────────────┤
//	│  Workspaces     │      Status Bar              │
//	└─────────────────┴──────────────────────────────┘
func (w *MainWindow) SetContent() {
	// Left side browser area: connection bar + service browser + workspaces
	browserWithWorkspace := container.NewVSplit(
		w.serviceBrowser, // top (service tree)
		w.workspacePanel, // bottom (workspace management)
	)
	browserWithWorkspace.SetOffset(0.7) // 70% services, 30% workspaces

	leftPanel := container.NewBorder(
		w.connectionBar,      // top (connection controls)
		nil,                  // bottom
		nil,                  // left
		nil,                  // right
		browserWithWorkspace, // center (browser + workspaces)
	)

	// Bottom bar: status on left, theme selector on right
	bottomBar := container.NewBorder(
		nil, nil, // top, bottom
		w.statusBar, nil, // left (status), right
		w.themeSelector, // center (theme selector pushed right)
	)

	// Right side: vertical split with request, response, and bottom bar
	rightPanel := container.NewBorder(
		nil,       // top
		bottomBar, // bottom (status bar + theme selector)
		nil,       // left
		nil,       // right
		container.NewVSplit(
			w.requestPanel,  // top half
			w.responsePanel, // bottom half
		),
	)

	// Main layout: horizontal split with browser on left, panels on right
	mainSplit := container.NewHSplit(
		leftPanel,  // left side (connection + browser)
		rightPanel, // right side (request/response/status)
	)

	// Set the initial split position (30% for browser, 70% for panels)
	mainSplit.SetOffset(0.3)

	w.window.SetContent(mainSplit)
}

// Window returns the underlying Fyne window.
func (w *MainWindow) Window() fyne.Window {
	return w.window
}

// handleClientStreamSend sends a single message in a client streaming RPC.
// This is called when the user clicks "Send Message" in the streaming input widget.
// On the first call, it starts the client stream. Subsequent calls send messages on the existing stream.
func (w *MainWindow) handleClientStreamSend(jsonStr string, metadataMap map[string]string) {
	// Get selected method
	serviceName, _ := w.state.SelectedService.Get()
	methodName, _ := w.state.SelectedMethod.Get()

	if serviceName == "" || methodName == "" {
		dialog.ShowError(fmt.Errorf("no method selected"), w.window)
		return
	}

	// If we don't have an active stream, start one
	if w.clientStreamHandle == nil {
		// Get method descriptor
		refClient := w.app.ReflectionClient()
		if refClient == nil {
			dialog.ShowError(fmt.Errorf("reflection client not initialized"), w.window)
			return
		}

		methodDesc, err := refClient.GetMethodDescriptor(serviceName, methodName)
		if err != nil {
			w.logger.Error("failed to get method descriptor", slog.Any("error", err))
			uierrors.ShowGRPCError(err, w.window, nil)
			return
		}

		// Verify this is a client streaming method
		if !methodDesc.IsClientStreaming() {
			dialog.ShowError(fmt.Errorf("method %s is not a client streaming RPC", methodName), w.window)
			return
		}

		// Convert metadata map to grpc metadata
		md := metadata.New(metadataMap)

		// Start the client stream
		invoker := w.app.Invoker()
		if invoker == nil {
			dialog.ShowError(fmt.Errorf("invoker not initialized"), w.window)
			return
		}

		ctx := context.Background()
		handle, err := invoker.InvokeClientStream(ctx, methodDesc, md)
		if err != nil {
			w.logger.Error("failed to start client stream", slog.Any("error", err))
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - attempt to start stream again
				w.handleClientStreamSend(jsonStr, metadataMap)
			})
			return
		}

		w.clientStreamHandle = handle
		w.logger.Info("client stream started",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)
	}

	// Send message on the stream
	if err := w.clientStreamHandle.Send(jsonStr); err != nil {
		w.logger.Error("failed to send client stream message", slog.Any("error", err))
		uierrors.ShowGRPCError(err, w.window, func() {
			// Retry callback - attempt to send the message again
			w.handleClientStreamSend(jsonStr, metadataMap)
		})
		// Clean up handle on error
		w.clientStreamHandle = nil
		return
	}

	w.logger.Debug("client stream message sent",
		slog.String("method", methodName),
	)
}

// handleClientStreamFinish closes the client stream and receives the final response.
// This is called when the user clicks "Finish & Get Response" in the streaming input widget.
func (w *MainWindow) handleClientStreamFinish(metadataMap map[string]string) {
	if w.clientStreamHandle == nil {
		// No active stream - start one if we haven't sent any messages yet
		// This allows "Finish & Get Response" to work even without sending messages
		w.handleClientStreamSend("{}", metadataMap)
		if w.clientStreamHandle == nil {
			// Failed to start stream
			return
		}
	}

	go func() {
		serviceName, _ := w.state.SelectedService.Get()
		methodName, _ := w.state.SelectedMethod.Get()

		w.logger.Info("closing client stream and receiving response",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)

		// Set loading state
		_ = w.state.Response.Loading.Set(true)
		_ = w.state.Response.Error.Set("")

		startTime := time.Now()

		// Close stream and receive response
		respJSON, err := w.clientStreamHandle.CloseAndReceive()

		duration := time.Since(startTime)
		_ = w.state.Response.Loading.Set(false)

		// Clean up handle
		w.clientStreamHandle = nil

		if err != nil {
			w.logger.Error("client stream failed", slog.Any("error", err))

			// Show rich gRPC error dialog
			uierrors.ShowGRPCError(err, w.window, nil)

			// Also set error in response panel for inline visibility
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

		w.logger.Info("client stream completed successfully",
			slog.String("method", methodName),
			slog.Duration("duration", duration),
		)
	}()
}

// captureWorkspaceState captures the current UI state into a Workspace
func (w *MainWindow) captureWorkspaceState() domain.Workspace {
	workspace := domain.Workspace{
		Connections: []domain.Connection{},
		Requests:    []domain.SavedRequest{},
	}

	// Capture current connection settings
	if address, _ := w.state.CurrentServer.Get(); address != "" {
		tlsSettings := w.connectionBar.GetTLSSettings()
		workspace.CurrentConnection = &domain.Connection{
			Address: address,
			UseTLS:  tlsSettings.Enabled,
			TLS:     tlsSettings,
		}
	}

	// Capture current request
	if requestBody, _ := w.state.Request.TextData.Get(); requestBody != "" {
		selectedMethod, _ := w.state.SelectedMethod.Get()

		// Get metadata from request panel
		metadataList, _ := w.state.Request.Metadata.Get()
		metadata := make(map[string]string)
		for i := 0; i < len(metadataList); i += 2 {
			if i+1 < len(metadataList) {
				metadata[metadataList[i]] = metadataList[i+1]
			}
		}

		workspace.CurrentRequest = &domain.Request{
			Method:   selectedMethod,
			Body:     requestBody,
			Metadata: metadata,
		}
	}

	// Capture selected service/method
	workspace.SelectedService, _ = w.state.SelectedService.Get()
	workspace.SelectedMethod, _ = w.state.SelectedMethod.Get()

	return workspace
}

// applyWorkspaceState applies a loaded workspace to the UI
func (w *MainWindow) applyWorkspaceState(workspace domain.Workspace) {
	w.logger.Info("applying workspace state", slog.String("workspace", workspace.Name))

	// Restore connection if saved
	if workspace.CurrentConnection != nil {
		conn := workspace.CurrentConnection
		w.connectionBar.SetAddress(conn.Address)
		w.connectionBar.SetTLSSettings(conn.TLS)
	}

	// Restore request if saved
	if workspace.CurrentRequest != nil {
		req := workspace.CurrentRequest
		_ = w.state.Request.TextData.Set(req.Body)

		// Convert metadata map to string list for binding
		metadataList := []string{}
		for key, value := range req.Metadata {
			metadataList = append(metadataList, key, value)
		}
		_ = w.state.Request.Metadata.Set(metadataList)
	}

	// Restore selected service/method
	if workspace.SelectedService != "" {
		_ = w.state.SelectedService.Set(workspace.SelectedService)
	}
	if workspace.SelectedMethod != "" {
		_ = w.state.SelectedMethod.Set(workspace.SelectedMethod)
	}

	w.logger.Info("workspace state applied successfully")
}

// switchToBidiPanel switches the right panel to show the bidi streaming UI
func (w *MainWindow) switchToBidiPanel() {
	// Update the window content to show bidi panel instead of request/response panels
	leftPanel := container.NewBorder(
		w.connectionBar,
		nil, nil, nil,
		w.serviceBrowser,
	)

	// Bottom bar: status on left, theme selector on right
	bottomBar := container.NewBorder(
		nil, nil, // top, bottom
		w.statusBar, nil, // left (status), right
		w.themeSelector, // center (theme selector pushed right)
	)

	rightPanel := container.NewBorder(
		nil,
		bottomBar,
		nil, nil,
		w.bidiPanel,
	)

	mainSplit := container.NewHSplit(leftPanel, rightPanel)
	mainSplit.SetOffset(0.3)
	w.window.SetContent(mainSplit)
}

// switchToNormalPanel switches back to normal request/response panel layout
func (w *MainWindow) switchToNormalPanel() {
	// Clean up any active bidi stream
	if w.bidiStreamHandle != nil {
		if w.bidiCancelFunc != nil {
			w.bidiCancelFunc()
		}
		w.bidiStreamHandle = nil
		w.bidiCancelFunc = nil
	}

	// Reset to original layout
	w.SetContent()
}

// handleBidiStreamSend sends a message on a bidirectional stream
func (w *MainWindow) handleBidiStreamSend(jsonStr string, metadataMap map[string]string) {
	serviceName, _ := w.state.SelectedService.Get()
	methodName, _ := w.state.SelectedMethod.Get()

	if serviceName == "" || methodName == "" {
		dialog.ShowError(fmt.Errorf("no method selected"), w.window)
		return
	}

	// If no active stream, start one
	if w.bidiStreamHandle == nil {
		refClient := w.app.ReflectionClient()
		if refClient == nil {
			dialog.ShowError(fmt.Errorf("reflection client not initialized"), w.window)
			return
		}

		methodDesc, err := refClient.GetMethodDescriptor(serviceName, methodName)
		if err != nil {
			w.logger.Error("failed to get method descriptor", slog.Any("error", err))
			uierrors.ShowGRPCError(err, w.window, nil)
			return
		}

		// Verify this is a bidi streaming method
		if !methodDesc.IsClientStreaming() || !methodDesc.IsServerStreaming() {
			dialog.ShowError(fmt.Errorf("method %s is not a bidirectional streaming RPC", methodName), w.window)
			return
		}

		// Convert metadata map to grpc metadata
		md := metadata.New(metadataMap)

		// Start the bidi stream
		invoker := w.app.Invoker()
		if invoker == nil {
			dialog.ShowError(fmt.Errorf("invoker not initialized"), w.window)
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		w.bidiCancelFunc = cancel

		handle, err := invoker.InvokeBidiStream(ctx, methodDesc, md)
		if err != nil {
			w.logger.Error("failed to start bidi stream", slog.Any("error", err))
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - attempt to start stream again
				w.handleBidiStreamSend(jsonStr, metadataMap)
			})
			w.bidiCancelFunc = nil
			return
		}

		w.bidiStreamHandle = handle
		w.logger.Info("bidi stream started",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)

		// Start receive goroutine
		go w.receiveBidiMessages()

		w.bidiPanel.SetStatus("Stream active")
	}

	// Send message on the stream
	if err := w.bidiStreamHandle.Send(jsonStr); err != nil {
		w.logger.Error("failed to send bidi stream message", slog.Any("error", err))
		w.bidiPanel.SetStatus(fmt.Sprintf("Send error: %s", err.Error()))
		w.bidiPanel.DisableSendControls()
		// Clean up handle on error
		if w.bidiCancelFunc != nil {
			w.bidiCancelFunc()
		}
		w.bidiStreamHandle = nil
		w.bidiCancelFunc = nil
		return
	}

	w.logger.Debug("bidi stream message sent", slog.String("method", methodName))
}

// receiveBidiMessages receives messages from the bidi stream in a background goroutine
func (w *MainWindow) receiveBidiMessages() {
	methodName, _ := w.state.SelectedMethod.Get()

	messageCount := 0
	for {
		jsonMsg, err := w.bidiStreamHandle.Recv()

		if err == io.EOF {
			w.logger.Info("bidi stream receive completed",
				slog.String("method", methodName),
				slog.Int("message_count", messageCount),
			)
			w.bidiPanel.SetStatus(fmt.Sprintf("Receive complete (%d messages)", messageCount))
			return
		}

		if err != nil {
			w.logger.Error("bidi stream receive error",
				slog.String("method", methodName),
				slog.Int("message_count", messageCount),
				slog.Any("error", err),
			)
			w.bidiPanel.SetStatus(fmt.Sprintf("Receive error: %s", err.Error()))
			w.bidiPanel.DisableSendControls()
			return
		}

		messageCount++

		// Pretty-print JSON message
		var prettyJSON interface{}
		if err := json.Unmarshal([]byte(jsonMsg), &prettyJSON); err == nil {
			prettyBytes, _ := json.MarshalIndent(prettyJSON, "", "  ")
			jsonMsg = string(prettyBytes)
		}

		// Add message to UI (bindings are thread-safe)
		w.bidiPanel.AddReceived(jsonMsg)

		w.logger.Debug("received bidi stream message",
			slog.String("method", methodName),
			slog.Int("message_num", messageCount),
		)
	}
}

// handleBidiStreamClose closes the send side of the bidi stream
func (w *MainWindow) handleBidiStreamClose() {
	if w.bidiStreamHandle == nil {
		w.logger.Warn("no active bidi stream to close")
		return
	}

	methodName, _ := w.state.SelectedMethod.Get()

	w.logger.Info("closing bidi stream send side",
		slog.String("method", methodName),
	)

	if err := w.bidiStreamHandle.CloseSend(); err != nil {
		w.logger.Error("failed to close bidi stream send side", slog.Any("error", err))
		w.bidiPanel.SetStatus(fmt.Sprintf("Close send error: %s", err.Error()))
		return
	}

	w.logger.Info("bidi stream send side closed",
		slog.String("method", methodName),
	)

	w.bidiPanel.SetStatus("Send closed (still receiving)")
}

// recordHistoryEntry saves a request/response to history
func (w *MainWindow) recordHistoryEntry(address, method, requestJSON string, requestMetadata map[string]string, responseJSON string, responseMetadata metadata.MD, duration time.Duration, err error) {
	// Get current connection settings
	currentConn := domain.Connection{
		Address: address,
	}
	if w.connectionBar != nil {
		currentConn.TLS = w.connectionBar.GetTLSSettings()
		currentConn.UseTLS = currentConn.TLS.Enabled
	}

	// Convert response metadata to map
	respMeta := make(map[string]string)
	for key, values := range responseMetadata {
		if len(values) > 0 {
			respMeta[key] = values[0]
			if len(values) > 1 {
				for i := 1; i < len(values); i++ {
					respMeta[key] += ", " + values[i]
				}
			}
		}
	}

	// Determine status
	status := "success"
	errorMsg := ""
	if err != nil {
		status = "error"
		errorMsg = err.Error()
	}

	// Create history entry
	entry := domain.HistoryEntry{
		ID:         history.GenerateEntryID(),
		Timestamp:  time.Now(),
		Connection: currentConn,
		Method:     method,
		Request:    requestJSON,
		Response:   responseJSON,
		Duration:   duration,
		Status:     status,
		Error:      errorMsg,
		Metadata: domain.Metadata{
			Request:  requestMetadata,
			Response: respMeta,
		},
	}

	// Save to history (non-blocking)
	go func() {
		if err := w.historyPanel.AddEntry(entry); err != nil {
			w.logger.Error("failed to save history entry", slog.Any("error", err))
		}
	}()
}

// handleHistoryReplay replays a request from history
func (w *MainWindow) handleHistoryReplay(entry domain.HistoryEntry) {
	w.logger.Info("replaying history entry",
		slog.String("id", entry.ID),
		slog.String("method", entry.Method),
	)

	// Connect to the server if not already connected
	currentServer, _ := w.state.CurrentServer.Get()
	if currentServer != entry.Connection.Address {
		w.logger.Info("connecting to historical server", slog.String("address", entry.Connection.Address))
		w.connectionBar.SetAddress(entry.Connection.Address)
		w.connectionBar.SetTLSSettings(entry.Connection.TLS)
		w.handleConnect(entry.Connection.Address, entry.Connection.TLS)

		// Give connection time to establish
		// In a real implementation, we'd wait for the connection state callback
		time.Sleep(2 * time.Second)
	}

	// Parse the method to extract service and method names
	// Format: "package.Service/Method"
	parts := strings.Split(entry.Method, "/")
	if len(parts) != 2 {
		w.logger.Error("invalid method format in history entry", slog.String("method", entry.Method))
		dialog.ShowError(fmt.Errorf("invalid method format: %s", entry.Method), w.window)
		return
	}

	serviceName := parts[0]
	methodName := parts[1]

	// Set selected service and method
	_ = w.state.SelectedService.Set(serviceName)
	_ = w.state.SelectedMethod.Set(methodName)

	// Set request body
	_ = w.state.Request.TextData.Set(entry.Request)

	// Set request metadata
	metadataList := []string{}
	for key, value := range entry.Metadata.Request {
		metadataList = append(metadataList, key, value)
	}
	_ = w.state.Request.Metadata.Set(metadataList)

	// Refresh UI to show the replayed request
	w.serviceBrowser.Refresh()

	w.logger.Info("history entry loaded into request panel - ready to send")
}

// setupMainMenu creates and sets the application's main menu
func (w *MainWindow) setupMainMenu() {
	// File menu - workspace operations
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Save Workspace", func() {
			w.workspacePanel.TriggerSave()
		}),
		fyne.NewMenuItem("Load Workspace", func() {
			w.workspacePanel.TriggerLoad()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Clear History", func() {
			w.handleClearHistory()
		}),
	)

	// Edit menu - clear operations
	editMenu := fyne.NewMenu("Edit",
		fyne.NewMenuItem("Clear Request", func() {
			w.handleClearRequest()
		}),
		fyne.NewMenuItem("Clear Response", func() {
			w.handleClearResponse()
		}),
	)

	// View menu - mode switching
	viewMenu := fyne.NewMenu("View",
		fyne.NewMenuItem("Text Mode", func() {
			w.requestPanel.SwitchToTextMode()
		}),
		fyne.NewMenuItem("Form Mode", func() {
			w.requestPanel.SwitchToFormMode()
		}),
	)

	// Help menu - about dialog
	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("About Grotto", func() {
			ShowAboutDialog(w.window)
		}),
	)

	// Create and set the main menu
	mainMenu := fyne.NewMainMenu(
		fileMenu,
		editMenu,
		viewMenu,
		helpMenu,
	)

	w.window.SetMainMenu(mainMenu)
}

// handleClearHistory shows a confirmation dialog and clears history if confirmed
func (w *MainWindow) handleClearHistory() {
	dialog.ShowConfirm("Clear History",
		"Are you sure you want to clear all request history?",
		func(confirmed bool) {
			if confirmed {
				if err := w.historyPanel.ClearHistory(); err != nil {
					w.logger.Error("failed to clear history", slog.Any("error", err))
					dialog.ShowError(fmt.Errorf("failed to clear history: %w", err), w.window)
				} else {
					w.logger.Info("history cleared")
				}
			}
		},
		w.window,
	)
}

// handleClearRequest clears the request panel
func (w *MainWindow) handleClearRequest() {
	_ = w.state.Request.TextData.Set("")
	_ = w.state.Request.Metadata.Set([]string{})
	w.logger.Debug("request panel cleared")
}

// handleClearResponse clears the response panel
func (w *MainWindow) handleClearResponse() {
	_ = w.state.Response.TextData.Set("")
	_ = w.state.Response.Error.Set("")
	_ = w.state.Response.Duration.Set("")
	w.responsePanel.ClearResponseMetadata()
	w.logger.Debug("response panel cleared")
}
