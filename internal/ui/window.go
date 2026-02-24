package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
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

	// Streaming state (protected by streamMu)
	streamMu           sync.Mutex
	clientStreamHandle *grpc.ClientStreamHandle
	clientStreamCancel context.CancelFunc
	bidiStreamHandle   *grpc.BidiStreamHandle
	bidiCancelFunc     context.CancelFunc
	serverStreamCancel context.CancelFunc
	unaryCancel        context.CancelFunc

	// Layout state
	inBidiMode   bool             // avoid unnecessary rebuilds
	contentSplit *container.Split // request/response vertical split (stored for offset changes)

	// Per-method request cache: "service/method" → last JSON text
	methodRequestCache map[string]string
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
		window:             window,
		state:              app.State(),
		logger:             app.Logger(),
		app:                app,
		connState:          connState,
		methodRequestCache: make(map[string]string),
	}

	// Create real UI components
	mw.connectionBar = browser.NewConnectionBar(connState, window, app.Storage())
	mw.serviceBrowser = browser.NewServiceBrowser(mw.state.Services)
	mw.requestPanel = request.NewRequestPanel(mw.state.Request, mw.logger)
	mw.responsePanel = response.NewResponsePanel(mw.state.Response, window)
	mw.bidiPanel = bidi.NewBidiStreamPanel(window)
	mw.statusBar = uierrors.NewStatusBar(connState)
	mw.workspacePanel = workspace.NewWorkspacePanel(app.Storage(), app.Logger(), window)
	mw.historyPanel = history.NewHistoryPanel(app.Storage(), app.Logger(), window)
	mw.themeSelector = CreateThemeSelector(fyneApp)

	// Wire up callbacks
	mw.wireCallbacks()

	// Set up the window content
	mw.SetContent()

	// Set up the main menu
	mw.setupMainMenu()

	// Set up keyboard shortcuts
	mw.setupKeyboardShortcuts()

	// Cancel all streams on window close
	window.SetCloseIntercept(func() {
		mw.cancelAllStreams()
		window.Close()
	})

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

	// Error service selection — show reflection error in response panel
	w.serviceBrowser.SetOnServiceError(func(service domain.Service) {
		_ = w.state.Response.Error.Set(
			fmt.Sprintf("Service %s failed reflection:\n%s", service.FullName, service.Error))
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

	// History: click to load (without sending)
	w.historyPanel.SetOnSelect(func(entry domain.HistoryEntry) {
		w.handleHistoryLoad(entry)
	})

	// History: replay (connect + load + send)
	w.historyPanel.SetOnReplay(func(entry domain.HistoryEntry) {
		w.handleHistoryReplay(entry)
	})
}

// formatByteSize returns a human-readable byte count (e.g., "1.2 KB", "3.4 MB").
func formatByteSize(bytes int) string {
	const (
		kb = 1024
		mb = kb * 1024
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// convertMetadataToMap converts gRPC metadata.MD to a flat map[string]string.
func convertMetadataToMap(md metadata.MD) map[string]string {
	result := make(map[string]string)
	for key, values := range md {
		if len(values) > 0 {
			result[key] = values[0]
			for i := 1; i < len(values); i++ {
				result[key] += ", " + values[i]
			}
		}
	}
	return result
}

// handleConnect establishes a connection and lists services
func (w *MainWindow) handleConnect(address string, tlsSettings domain.TLSSettings) {
	go func() {
		ctx := context.Background()

		// Update UI state (bindings are thread-safe)
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

			// Show rich gRPC error dialog with retry option (must be on main thread)
			fyne.Do(func() {
				uierrors.ShowGRPCError(err, w.window, func() {
					// Retry callback - attempt connection again
					w.handleConnect(address, tlsSettings)
				})
			})
			return
		}

		// Initialize reflection client
		if err := w.app.InitializeReflectionClient(); err != nil {
			w.logger.Error("failed to initialize reflection", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to initialize reflection: " + err.Error())

			// Show rich gRPC error dialog with retry option (must be on main thread)
			fyne.Do(func() {
				uierrors.ShowGRPCError(err, w.window, func() {
					// Retry callback - attempt connection again
					w.handleConnect(address, tlsSettings)
				})
			})
			return
		}

		// List services
		services, err := w.app.ReflectionClient().ListServices(ctx)
		if err != nil {
			w.logger.Error("failed to list services", slog.Any("error", err))
			_ = w.connState.State.Set("error")
			_ = w.connState.Message.Set("Failed to list services: " + err.Error())

			// Show rich gRPC error dialog with retry option (must be on main thread)
			fyne.Do(func() {
				uierrors.ShowGRPCError(err, w.window, func() {
					// Retry callback - attempt connection again
					w.handleConnect(address, tlsSettings)
				})
			})
			return
		}

		// Update state with services (bindings are thread-safe)
		servicesInterface := make([]interface{}, len(services))
		for i, svc := range services {
			servicesInterface[i] = svc
		}
		_ = w.state.Services.Set(servicesInterface)

		// Update connection state (bindings are thread-safe)
		_ = w.state.CurrentServer.Set(address)
		_ = w.state.Connected.Set(true)
		_ = w.connState.State.Set("connected")

		// Status message: include error count when some services failed
		var errorCount int
		for _, svc := range services {
			if svc.Error != "" {
				errorCount++
			}
		}
		statusMsg := "Connected to " + address
		if errorCount > 0 {
			statusMsg = fmt.Sprintf("Connected to %s (%d services, %d with errors)",
				address, len(services), errorCount)
		}
		_ = w.connState.Message.Set(statusMsg)

		w.logger.Info("connection established and services loaded",
			slog.String("address", address),
			slog.Int("service_count", len(services)),
		)

		// Save to recent connections
		w.connectionBar.SaveConnection(cfg)

		// Refresh the service browser and focus it (must be on main thread in Fyne 2.6+)
		fyne.Do(func() {
			w.serviceBrowser.Refresh()
			w.serviceBrowser.FocusTree()
		})
	}()
}

// cancelAllStreams cancels all active stream operations and clears their handles.
// Cancel funcs are called outside the lock to avoid potential deadlocks.
func (w *MainWindow) cancelAllStreams() {
	w.streamMu.Lock()
	unaryCancel := w.unaryCancel
	w.unaryCancel = nil
	serverCancel := w.serverStreamCancel
	w.serverStreamCancel = nil
	bidiCancel := w.bidiCancelFunc
	w.bidiCancelFunc = nil
	w.bidiStreamHandle = nil
	clientCancel := w.clientStreamCancel
	w.clientStreamCancel = nil
	clientHandle := w.clientStreamHandle
	w.clientStreamHandle = nil
	w.streamMu.Unlock()

	// Call cancel funcs outside the lock
	if unaryCancel != nil {
		unaryCancel()
	}
	if serverCancel != nil {
		serverCancel()
	}
	if bidiCancel != nil {
		bidiCancel()
	}
	if clientCancel != nil {
		clientCancel()
	}
	if clientHandle != nil {
		// CloseAndReceive blocks, so run in goroutine
		go clientHandle.CloseAndReceive()
	}
}

// handleDisconnect closes the connection
func (w *MainWindow) handleDisconnect() {
	// Cancel all active streams before disconnecting
	w.cancelAllStreams()
	if w.inBidiMode {
		w.switchToNormalPanel()
	}

	go func() {
		// Clean up reflection client
		w.app.CleanupReflectionClient()

		// Disconnect
		if err := w.app.ConnManager().Disconnect(); err != nil {
			w.logger.Error("disconnect failed", slog.Any("error", err))
			fyne.Do(func() {
				dialog.ShowError(err, w.window)
			})
			return
		}

		// Clear UI state (bindings are thread-safe)
		_ = w.state.Services.Set([]interface{}{})
		_ = w.state.Connected.Set(false)
		_ = w.state.CurrentServer.Set("")
		_ = w.state.SelectedService.Set("")
		_ = w.state.SelectedMethod.Set("")
		w.requestPanel.SetSendEnabled(false)
		w.methodRequestCache = make(map[string]string)

		// Update connection state to reflect disconnection
		_ = w.connState.State.Set("disconnected")
		_ = w.connState.Message.Set("Disconnected")

		// Refresh the service browser to clear the tree (must be on main thread)
		fyne.Do(func() {
			w.serviceBrowser.Refresh()
		})

		w.logger.Info("disconnected")
	}()
}

// handleMethodSelect updates the UI when a method is selected
func (w *MainWindow) handleMethodSelect(service domain.Service, method domain.Method) {
	// Cancel any active streams before switching methods
	w.cancelAllStreams()

	w.logger.Debug("method selected",
		slog.String("service", service.FullName),
		slog.String("method", method.Name),
	)

	// Cache the current method's request JSON before switching
	prevService, _ := w.state.SelectedService.Get()
	prevMethod, _ := w.state.SelectedMethod.Get()
	if prevService != "" && prevMethod != "" {
		currentJSON, _ := w.state.Request.TextData.Get()
		if currentJSON != "" {
			w.methodRequestCache[prevService+"/"+prevMethod] = currentJSON
		}
	}

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

	// v2 descriptors are already stdlib protoreflect types
	protoDesc := methodDesc.Input()

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
		w.bidiPanel.SetOnAbort(func() {
			w.streamMu.Lock()
			bidiCancel := w.bidiCancelFunc
			w.bidiCancelFunc = nil
			w.bidiStreamHandle = nil
			w.streamMu.Unlock()
			if bidiCancel != nil {
				bidiCancel()
			}
		})
		w.bidiPanel.SetStatus("Ready to start bidirectional stream")
	} else {
		// For other method types, use normal request/response panels
		w.switchToNormalPanel()

		// Update request panel with method descriptor
		w.requestPanel.SetMethod(method.Name, protoDesc)
		w.requestPanel.SetSendEnabled(true)

		// Restore cached request JSON for this method (if any)
		cacheKey := service.FullName + "/" + method.Name
		if cached, ok := w.methodRequestCache[cacheKey]; ok {
			_ = w.state.Request.TextData.Set(cached)
			w.requestPanel.SyncTextToForm()
		}

		// Set client streaming mode based on method type
		w.requestPanel.SetClientStreaming(method.IsClientStream)

		// Clear previous response
		_ = w.state.Response.TextData.Set("")
		_ = w.state.Response.Error.Set("")
		_ = w.state.Response.Duration.Set("")
		_ = w.state.Response.Size.Set("")
		w.responsePanel.ClearResponseMetadata()

		// Focus the request editor for immediate typing
		w.requestPanel.FocusEditor()
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
	if methodDesc.IsStreamingServer() {
		w.handleServerStreamRequest(jsonStr, metadataMap, methodDesc)
	} else {
		w.handleUnaryRequest(jsonStr, metadataMap, methodDesc)
	}
}

// handleUnaryRequest handles unary RPC invocations
func (w *MainWindow) handleUnaryRequest(jsonStr string, metadataMap map[string]string, methodDesc protoreflect.MethodDescriptor) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		w.streamMu.Lock()
		w.unaryCancel = cancel
		w.streamMu.Unlock()

		serviceName, _ := w.state.SelectedService.Get()
		methodName, _ := w.state.SelectedMethod.Get()

		w.logger.Debug("sending unary request",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)

		// Set loading state and switch to normal response mode
		_ = w.state.Response.Loading.Set(true)
		_ = w.state.Response.Error.Set("")
		fyne.Do(func() {
			w.responsePanel.SetStreaming(false)
		})

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

		respJSON, respHeaders, respTrailers, err := invoker.InvokeUnary(ctx, methodDesc, jsonStr, md)

		duration := time.Since(startTime)
		_ = w.state.Response.Loading.Set(false)

		// Record history entry
		currentServer, _ := w.state.CurrentServer.Get()
		w.recordHistoryEntry(currentServer, serviceName+"/"+methodName, jsonStr, metadataMap, respJSON, respHeaders, duration, err)

		if err != nil {
			w.logger.Error("RPC invocation failed", slog.Any("error", err))

			// Show rich gRPC error dialog with retry option (must be on main thread)
			fyne.Do(func() {
				uierrors.ShowGRPCError(err, w.window, func() {
					// Retry callback - send the request again
					w.handleSendRequest(jsonStr, metadataMap)
				})
				w.responsePanel.ClearResponseMetadata()
				w.expandResponsePanel()
			})

			// Also set error in response panel for inline visibility
			_ = w.state.Response.Error.Set(err.Error())
			return
		}

		// Pretty-print JSON response
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(respJSON), "", "  "); err == nil {
			respJSON = buf.String()
		}

		// Convert metadata to maps for display
		respMetadataMap := convertMetadataToMap(respHeaders)
		respTrailersMap := convertMetadataToMap(respTrailers)

		// Update response (bindings are thread-safe, but widget methods need main thread)
		_ = w.state.Response.TextData.Set(respJSON)
		_ = w.state.Response.Duration.Set(fmt.Sprintf("Duration: %v", duration.Round(time.Millisecond)))
		_ = w.state.Response.Size.Set(formatByteSize(len(respJSON)))
		_ = w.state.Response.Error.Set("")

		fyne.Do(func() {
			w.responsePanel.SetResponseMetadata(respMetadataMap)
			w.responsePanel.SetResponseTrailers(respTrailersMap)
			w.expandResponsePanel()
		})

		w.logger.Info("RPC completed successfully",
			slog.String("method", methodName),
			slog.Duration("duration", duration),
		)
	}()
}

// handleServerStreamRequest handles server streaming RPC invocations
func (w *MainWindow) handleServerStreamRequest(jsonStr string, metadataMap map[string]string, methodDesc protoreflect.MethodDescriptor) {
	ctx, cancel := context.WithCancel(context.Background())
	w.streamMu.Lock()
	w.serverStreamCancel = cancel
	w.streamMu.Unlock()

	serviceName, _ := w.state.SelectedService.Get()
	methodName, _ := w.state.SelectedMethod.Get()

	w.logger.Debug("sending server stream request",
		slog.String("service", serviceName),
		slog.String("method", methodName),
	)

	// Switch to streaming mode and prepare UI
	w.responsePanel.SetStreaming(true)
	w.expandResponsePanel()
	streamWidget := w.responsePanel.StreamingWidget()
	streamWidget.Clear()
	streamWidget.SetStatus("Starting stream...")
	streamWidget.EnableStopButton()

	// Set stop button handler
	streamWidget.SetOnStop(func() {
		w.logger.Info("user requested stream stop")
		cancel()
		w.streamMu.Lock()
		w.serverStreamCancel = nil
		w.streamMu.Unlock()
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
	msgChan, errChan, headerChan, trailerChan := invoker.InvokeServerStream(ctx, methodDesc, jsonStr, md)

	// Process messages in a goroutine
	go func() {
		defer cancel() // ensure context is cleaned up on all exit paths
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
				var buf bytes.Buffer
				if err := json.Indent(&buf, []byte(jsonMsg), "", "  "); err == nil {
					jsonMsg = buf.String()
				}

				// Add message to UI (must be on main thread)
				fyne.Do(func() {
					streamWidget.AddMessage(jsonMsg)
				})

			case err, ok := <-errChan:
				if !ok {
					// Channel closed
					return
				}

				duration := time.Since(startTime)

				// Read trailers (sent before error by invoker)
				select {
				case trailers := <-trailerChan:
					trailersMap := convertMetadataToMap(trailers)
					fyne.Do(func() {
						w.responsePanel.SetResponseTrailers(trailersMap)
					})
				default:
				}

				// Record history for server streaming
				currentServer, _ := w.state.CurrentServer.Get()
				streamStatus := "success"
				streamErr := ""
				if err != io.EOF {
					streamStatus = "error"
					streamErr = err.Error()
				}
				go w.recordStreamHistoryEntry(currentServer, serviceName+"/"+methodName, jsonStr, metadataMap, duration, streamStatus, streamErr, "server_stream", messageCount)

				// Check if this is normal stream completion (io.EOF) or an error
				if err == io.EOF {
					w.logger.Info("server stream completed successfully",
						slog.String("method", methodName),
						slog.Int("message_count", messageCount),
						slog.Duration("duration", duration),
					)

					fyne.Do(func() {
						streamWidget.SetStatus(fmt.Sprintf("Complete (%d messages in %v)", messageCount, duration.Round(time.Millisecond)))
						streamWidget.DisableStopButton()
					})
				} else {
					w.logger.Error("server stream error",
						slog.String("method", methodName),
						slog.Int("message_count", messageCount),
						slog.Any("error", err),
					)

					fyne.Do(func() {
						streamWidget.SetStatus(fmt.Sprintf("Error: %s (received %d messages)", err.Error(), messageCount))
						streamWidget.DisableStopButton()
					})
				}

				return

			case hdr, ok := <-headerChan:
				if ok {
					hdrsMap := convertMetadataToMap(hdr)
					fyne.Do(func() {
						w.responsePanel.SetResponseMetadata(hdrsMap)
					})
				}
			}
		}
	}()
}

// SetContent builds and sets the main window layout.
// Layout structure:
//
//	┌──────────────────────────────────────────────────┐
//	│               Connection Bar                     │
//	├─────────────────┬────────────────────────────────┤
//	│                 │      Request Panel             │
//	│  Service        ├────────────────────────────────┤
//	│  Browser        │      Response Panel            │
//	├─────────────────┼────────────────────────────────┤
//	│  Workspaces     │      Status Bar                │
//	└─────────────────┴────────────────────────────────┘
//
// buildLeftPanel constructs the left panel with service browser and workspace tabs.
func (w *MainWindow) buildLeftPanel() *fyne.Container {
	leftTabs := container.NewAppTabs(
		container.NewTabItem("Workspaces", w.workspacePanel),
		container.NewTabItem("History", w.historyPanel),
	)
	browserWithTabs := container.NewVSplit(
		w.serviceBrowser,
		leftTabs,
	)
	browserWithTabs.SetOffset(0.7)
	return container.NewBorder(
		nil,
		nil, nil, nil,
		browserWithTabs,
	)
}

func (w *MainWindow) SetContent() {
	leftPanel := w.buildLeftPanel()

	// Bottom bar: status on left, theme selector on right
	bottomBar := container.NewBorder(
		nil, nil, // top, bottom
		w.statusBar, nil, // left (status), right
		w.themeSelector, // center (theme selector pushed right)
	)

	// Right side: vertical split with request, response, and bottom bar
	w.contentSplit = container.NewVSplit(
		w.requestPanel,  // top (gets most space initially)
		w.responsePanel, // bottom (minimized until first response)
	)
	w.contentSplit.SetOffset(0.75) // 75% request, 25% response
	rightPanel := container.NewBorder(
		nil,       // top
		bottomBar, // bottom (status bar + theme selector)
		nil,       // left
		nil,       // right
		w.contentSplit,
	)

	// Main layout: horizontal split with browser on left, panels on right
	mainSplit := container.NewHSplit(
		leftPanel,  // left side (browser + workspaces)
		rightPanel, // right side (request/response/status)
	)

	// Set the initial split position (30% for browser, 70% for panels)
	mainSplit.SetOffset(0.3)

	// Connection bar spans full window width above the split
	w.window.SetContent(container.NewBorder(w.connectionBar, nil, nil, nil, mainSplit))
}

// Window returns the underlying Fyne window.
func (w *MainWindow) Window() fyne.Window {
	return w.window
}

// expandResponsePanel sets the content split to give equal space to request/response.
func (w *MainWindow) expandResponsePanel() {
	if w.contentSplit != nil {
		w.contentSplit.SetOffset(0.5)
	}
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
	w.streamMu.Lock()
	needsNewStream := w.clientStreamHandle == nil
	w.streamMu.Unlock()
	if needsNewStream {
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
		if !methodDesc.IsStreamingClient() {
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

		ctx, cancel := context.WithCancel(context.Background())
		handle, err := invoker.InvokeClientStream(ctx, methodDesc, md)
		if err != nil {
			cancel()
			w.logger.Error("failed to start client stream", slog.Any("error", err))
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - attempt to start stream again
				w.handleClientStreamSend(jsonStr, metadataMap)
			})
			return
		}

		w.streamMu.Lock()
		w.clientStreamHandle = handle
		w.clientStreamCancel = cancel
		w.streamMu.Unlock()
		w.logger.Info("client stream started",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)
	}

	// Send message on the stream
	w.streamMu.Lock()
	csHandle := w.clientStreamHandle
	w.streamMu.Unlock()
	if csHandle == nil {
		w.logger.Error("client stream handle unexpectedly nil")
		return
	}
	if err := csHandle.Send(jsonStr); err != nil {
		w.logger.Error("failed to send client stream message", slog.Any("error", err))
		uierrors.ShowGRPCError(err, w.window, func() {
			// Retry callback - attempt to send the message again
			w.handleClientStreamSend(jsonStr, metadataMap)
		})
		// Clean up handle and cancel context on error
		w.streamMu.Lock()
		w.clientStreamHandle = nil
		sendErrCancel := w.clientStreamCancel
		w.clientStreamCancel = nil
		w.streamMu.Unlock()
		if sendErrCancel != nil {
			sendErrCancel()
		}
		return
	}

	w.logger.Debug("client stream message sent",
		slog.String("method", methodName),
	)
}

// handleClientStreamFinish closes the client stream and receives the final response.
// This is called when the user clicks "Finish & Get Response" in the streaming input widget.
func (w *MainWindow) handleClientStreamFinish(metadataMap map[string]string) {
	w.streamMu.Lock()
	hasStream := w.clientStreamHandle != nil
	w.streamMu.Unlock()
	if !hasStream {
		// No active stream - start one if we haven't sent any messages yet
		// This allows "Finish & Get Response" to work even without sending messages
		w.handleClientStreamSend("{}", metadataMap)
		w.streamMu.Lock()
		hasStream = w.clientStreamHandle != nil
		w.streamMu.Unlock()
		if !hasStream {
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
		w.streamMu.Lock()
		csHandle := w.clientStreamHandle
		w.streamMu.Unlock()
		if csHandle == nil {
			_ = w.state.Response.Loading.Set(false)
			_ = w.state.Response.Error.Set("Client stream was cancelled")
			return
		}
		respJSON, err := csHandle.CloseAndReceive()

		// Capture trailers (available after stream ends)
		csTrailers := csHandle.Trailer()

		duration := time.Since(startTime)
		_ = w.state.Response.Loading.Set(false)

		// Clean up handle and cancel func
		w.streamMu.Lock()
		w.clientStreamHandle = nil
		csCancel := w.clientStreamCancel
		w.clientStreamCancel = nil
		w.streamMu.Unlock()
		if csCancel != nil {
			csCancel()
		}

		// Record history
		currentServer, _ := w.state.CurrentServer.Get()
		w.recordHistoryEntry(currentServer, serviceName+"/"+methodName, "", metadataMap, respJSON, nil, duration, err)

		if err != nil {
			w.logger.Error("client stream failed", slog.Any("error", err))

			// Show rich gRPC error dialog (must be on main thread)
			fyne.Do(func() {
				uierrors.ShowGRPCError(err, w.window, nil)
			})

			// Also set error in response panel for inline visibility
			_ = w.state.Response.Error.Set(err.Error())
			return
		}

		// Capture headers
		if csHeaders, hdErr := csHandle.Header(); hdErr == nil {
			fyne.Do(func() {
				w.responsePanel.SetResponseMetadata(convertMetadataToMap(csHeaders))
			})
		}

		// Pretty-print JSON response
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(respJSON), "", "  "); err == nil {
			respJSON = buf.String()
		}

		// Update response
		_ = w.state.Response.TextData.Set(respJSON)
		_ = w.state.Response.Duration.Set(fmt.Sprintf("Duration: %v", duration.Round(time.Millisecond)))
		_ = w.state.Response.Size.Set(formatByteSize(len(respJSON)))
		_ = w.state.Response.Error.Set("")
		fyne.Do(func() {
			w.responsePanel.SetResponseTrailers(convertMetadataToMap(csTrailers))
			w.expandResponsePanel()
		})

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
	// Skip if already in bidi mode (avoid expensive layout rebuild)
	if w.inBidiMode {
		return
	}

	// Update the window content to show bidi panel instead of request/response panels
	leftPanel := w.buildLeftPanel()

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
	w.window.SetContent(container.NewBorder(w.connectionBar, nil, nil, nil, mainSplit))
	w.inBidiMode = true
}

// switchToNormalPanel switches back to normal request/response panel layout
func (w *MainWindow) switchToNormalPanel() {
	// Skip if already in normal mode (avoid expensive layout rebuild)
	if !w.inBidiMode {
		return
	}

	// Clean up any active streams
	w.cancelAllStreams()

	// Reset to original layout
	w.SetContent()
	w.inBidiMode = false
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
	w.streamMu.Lock()
	needsNewBidiStream := w.bidiStreamHandle == nil
	w.streamMu.Unlock()
	if needsNewBidiStream {
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
		if !methodDesc.IsStreamingClient() || !methodDesc.IsStreamingServer() {
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
		w.streamMu.Lock()
		w.bidiCancelFunc = cancel
		w.streamMu.Unlock()

		handle, err := invoker.InvokeBidiStream(ctx, methodDesc, md)
		if err != nil {
			w.logger.Error("failed to start bidi stream", slog.Any("error", err))
			uierrors.ShowGRPCError(err, w.window, func() {
				// Retry callback - attempt to start stream again
				w.handleBidiStreamSend(jsonStr, metadataMap)
			})
			w.streamMu.Lock()
			w.bidiCancelFunc = nil
			w.streamMu.Unlock()
			return
		}

		w.streamMu.Lock()
		w.bidiStreamHandle = handle
		w.streamMu.Unlock()
		w.logger.Info("bidi stream started",
			slog.String("service", serviceName),
			slog.String("method", methodName),
		)

		// Start receive goroutine
		go w.receiveBidiMessages()

		w.bidiPanel.SetStatus("Stream active")
	}

	// Send message on the stream
	w.streamMu.Lock()
	bidiHandle := w.bidiStreamHandle
	w.streamMu.Unlock()
	if bidiHandle == nil {
		w.logger.Error("bidi stream handle unexpectedly nil")
		return
	}
	if err := bidiHandle.Send(jsonStr); err != nil {
		w.logger.Error("failed to send bidi stream message", slog.Any("error", err))
		w.bidiPanel.SetStatus(fmt.Sprintf("Send error: %s", err.Error()))
		w.bidiPanel.DisableSendControls()
		// Clean up handle on error
		w.streamMu.Lock()
		bidiCancel := w.bidiCancelFunc
		w.bidiStreamHandle = nil
		w.bidiCancelFunc = nil
		w.streamMu.Unlock()
		if bidiCancel != nil {
			bidiCancel()
		}
		return
	}

	w.logger.Debug("bidi stream message sent", slog.String("method", methodName))
}

// receiveBidiMessages receives messages from the bidi stream in a background goroutine
func (w *MainWindow) receiveBidiMessages() {
	currentServer, _ := w.state.CurrentServer.Get()
	serviceName, _ := w.state.SelectedService.Get()
	methodName, _ := w.state.SelectedMethod.Get()

	w.streamMu.Lock()
	handle := w.bidiStreamHandle
	w.streamMu.Unlock()
	if handle == nil {
		w.logger.Warn("bidi stream handle nil at receive start")
		return
	}

	startTime := time.Now()
	messageCount := 0
	var streamErr error

	for {
		jsonMsg, err := handle.Recv()

		if err == io.EOF {
			w.logger.Info("bidi stream receive completed",
				slog.String("method", methodName),
				slog.Int("message_count", messageCount),
			)
			break
		}

		if err != nil {
			streamErr = err
			w.logger.Error("bidi stream receive error",
				slog.String("method", methodName),
				slog.Int("message_count", messageCount),
				slog.Any("error", err),
			)
			break
		}

		messageCount++

		// Pretty-print JSON message
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(jsonMsg), "", "  "); err == nil {
			jsonMsg = buf.String()
		}

		// Add message to UI (must be on main thread)
		fyne.Do(func() {
			w.bidiPanel.AddReceived(jsonMsg)
		})

		w.logger.Debug("received bidi stream message",
			slog.String("method", methodName),
			slog.Int("message_num", messageCount),
		)
	}

	duration := time.Since(startTime)

	// Capture trailers and headers
	trailers := handle.Trailer()
	headers, _ := handle.Header()

	// Update UI with final status, headers, and trailers
	fyne.Do(func() {
		if streamErr != nil {
			w.bidiPanel.SetStatus(fmt.Sprintf("Receive error: %s", streamErr.Error()))
			w.bidiPanel.DisableSendControls()
		} else {
			w.bidiPanel.SetStatus(fmt.Sprintf("Receive complete (%d messages)", messageCount))
		}

		// Display headers and trailers on the response panel
		if headers != nil {
			w.responsePanel.SetResponseMetadata(convertMetadataToMap(headers))
		}
		if trailers != nil {
			w.responsePanel.SetResponseTrailers(convertMetadataToMap(trailers))
		}
	})

	// Record history
	status := "OK"
	errorMsg := ""
	if streamErr != nil {
		status = "ERROR"
		errorMsg = streamErr.Error()
	}
	w.recordStreamHistoryEntry(currentServer, serviceName+"/"+methodName, "", nil, duration, status, errorMsg, "bidi_stream", messageCount)
}

// handleBidiStreamClose closes the send side of the bidi stream
func (w *MainWindow) handleBidiStreamClose() {
	w.streamMu.Lock()
	bidiHandle := w.bidiStreamHandle
	w.streamMu.Unlock()
	if bidiHandle == nil {
		w.logger.Warn("no active bidi stream to close")
		return
	}

	methodName, _ := w.state.SelectedMethod.Get()

	w.logger.Info("closing bidi stream send side",
		slog.String("method", methodName),
	)

	if err := bidiHandle.CloseSend(); err != nil {
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

// recordStreamHistoryEntry saves a streaming RPC summary to history.
func (w *MainWindow) recordStreamHistoryEntry(address, method, requestJSON string, requestMetadata map[string]string, duration time.Duration, status, errorMsg, streamType string, messageCount int) {
	currentConn := domain.Connection{
		Address: address,
	}
	if w.connectionBar != nil {
		currentConn.TLS = w.connectionBar.GetTLSSettings()
		currentConn.UseTLS = currentConn.TLS.Enabled
	}

	entry := domain.HistoryEntry{
		ID:           history.GenerateEntryID(),
		Timestamp:    time.Now(),
		Connection:   currentConn,
		Method:       method,
		Request:      requestJSON,
		Response:     fmt.Sprintf("(%d messages)", messageCount),
		Duration:     duration,
		Status:       status,
		Error:        errorMsg,
		StreamType:   streamType,
		MessageCount: messageCount,
		Metadata: domain.Metadata{
			Request: requestMetadata,
		},
	}

	if err := w.historyPanel.AddEntry(entry); err != nil {
		w.logger.Error("failed to save stream history entry", slog.Any("error", err))
	}
}

// handleHistoryLoad loads a history entry into the UI without sending.
// It sets the connection, selects the method, and fills request data.
func (w *MainWindow) handleHistoryLoad(entry domain.HistoryEntry) {
	w.logger.Info("loading history entry",
		slog.String("id", entry.ID),
		slog.String("method", entry.Method),
	)

	// Parse the method to extract service and method names
	// Format: "package.Service/Method"
	parts := strings.Split(entry.Method, "/")
	if len(parts) != 2 {
		w.logger.Error("invalid method format in history entry", slog.String("method", entry.Method))
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("invalid method format: %s", entry.Method), w.window)
		})
		return
	}

	serviceName := parts[0]
	methodName := parts[1]

	// afterConnect is called once we know the server is connected and services
	// are loaded. It selects the method, fills request data, and optionally sends.
	afterConnect := func(andSend bool) {
		fyne.Do(func() {
			// Select the method in the service browser — this triggers
			// handleMethodSelect which rebuilds the request form and proto descriptors.
			w.serviceBrowser.SelectMethod(serviceName, methodName)
		})

		// Set request body in a separate fyne.Do to ensure it runs AFTER
		// SelectMethod's OnSelected callback (which clears TextData via SetMethod).
		fyne.Do(func() {
			_ = w.state.Request.TextData.Set(entry.Request)

			// Set metadata on the request panel's internal bindings
			w.requestPanel.SetMetadata(entry.Metadata.Request)

			// Populate form from the loaded JSON text
			w.requestPanel.SyncTextToForm()

			w.logger.Info("history entry loaded into request panel")

			if andSend {
				w.requestPanel.TriggerSend()
			}
		})
	}

	// Check if we need to connect to a different server
	currentServer, _ := w.state.CurrentServer.Get()
	needsConnect := currentServer != entry.Connection.Address

	if needsConnect {
		w.logger.Info("connecting to historical server", slog.String("address", entry.Connection.Address))
		w.connectionBar.SetAddress(entry.Connection.Address)
		w.connectionBar.SetTLSSettings(entry.Connection.TLS)
		w.handleConnect(entry.Connection.Address, entry.Connection.TLS)

		// Wait for connection to complete by listening for state changes
		go func() {
			done := make(chan struct{})
			var listener binding.DataListener
			listener = binding.NewDataListener(func() {
				state, _ := w.connState.State.Get()
				switch state {
				case "connected":
					w.connState.State.RemoveListener(listener)
					close(done)
				case "error":
					w.connState.State.RemoveListener(listener)
					close(done)
				}
			})
			w.connState.State.AddListener(listener)

			// Timeout after 30 seconds
			select {
			case <-done:
				state, _ := w.connState.State.Get()
				if state == "connected" {
					afterConnect(false)
				} else {
					w.logger.Error("connection failed while loading history entry")
				}
			case <-time.After(30 * time.Second):
				w.connState.State.RemoveListener(listener)
				w.logger.Error("timed out waiting for connection while loading history entry")
			}
		}()
	} else {
		afterConnect(false)
	}
}

// handleHistoryReplay replays a request from history: connects, loads, and sends.
func (w *MainWindow) handleHistoryReplay(entry domain.HistoryEntry) {
	w.logger.Info("replaying history entry",
		slog.String("id", entry.ID),
		slog.String("method", entry.Method),
	)

	// Parse the method to extract service and method names
	parts := strings.Split(entry.Method, "/")
	if len(parts) != 2 {
		w.logger.Error("invalid method format in history entry", slog.String("method", entry.Method))
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("invalid method format: %s", entry.Method), w.window)
		})
		return
	}

	serviceName := parts[0]
	methodName := parts[1]

	// afterConnect selects the method, fills request data, and triggers send.
	afterConnect := func() {
		fyne.Do(func() {
			w.serviceBrowser.SelectMethod(serviceName, methodName)
			_ = w.state.Request.TextData.Set(entry.Request)
			w.requestPanel.SetMetadata(entry.Metadata.Request)
			w.requestPanel.SyncTextToForm()

			w.logger.Info("history entry loaded - triggering send")
			w.requestPanel.TriggerSend()
		})
	}

	// Check if we need to connect to a different server
	currentServer, _ := w.state.CurrentServer.Get()
	needsConnect := currentServer != entry.Connection.Address

	if needsConnect {
		w.logger.Info("connecting to historical server", slog.String("address", entry.Connection.Address))
		w.connectionBar.SetAddress(entry.Connection.Address)
		w.connectionBar.SetTLSSettings(entry.Connection.TLS)
		w.handleConnect(entry.Connection.Address, entry.Connection.TLS)

		// Wait for connection to complete by listening for state changes
		go func() {
			done := make(chan struct{})
			var listener binding.DataListener
			listener = binding.NewDataListener(func() {
				state, _ := w.connState.State.Get()
				switch state {
				case "connected":
					w.connState.State.RemoveListener(listener)
					close(done)
				case "error":
					w.connState.State.RemoveListener(listener)
					close(done)
				}
			})
			w.connState.State.AddListener(listener)

			select {
			case <-done:
				state, _ := w.connState.State.Get()
				if state == "connected" {
					afterConnect()
				} else {
					w.logger.Error("connection failed while replaying history entry")
				}
			case <-time.After(30 * time.Second):
				w.connState.State.RemoveListener(listener)
				w.logger.Error("timed out waiting for connection while replaying history entry")
			}
		}()
	} else {
		afterConnect()
	}
}

// toggleConnection toggles between connected and disconnected states.
func (w *MainWindow) toggleConnection() {
	state, _ := w.connState.State.Get()
	switch state {
	case "disconnected", "error":
		w.connectionBar.TriggerConnect()
	case "connected":
		w.handleDisconnect()
	}
}

// setupMainMenu creates and sets the application's main menu.
// Menu items that have keyboard shortcuts show the accelerator hint via MenuItem.Shortcut.
// Note: setting Shortcut on a MenuItem only displays the hint — shortcuts are still
// registered globally via canvas.AddShortcut in setupKeyboardShortcuts.
func (w *MainWindow) setupMainMenu() {
	// File menu - workspace and connection operations
	saveItem := fyne.NewMenuItem("Save Workspace", func() {
		w.workspacePanel.TriggerSave()
	})
	saveItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierSuper,
	}

	loadItem := fyne.NewMenuItem("Load Workspace", func() {
		w.workspacePanel.TriggerLoad()
	})
	loadItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierSuper,
	}

	connectItem := fyne.NewMenuItem("Connect / Disconnect", func() {
		w.toggleConnection()
	})
	connectItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyC,
		Modifier: fyne.KeyModifierSuper | fyne.KeyModifierShift,
	}

	fileMenu := fyne.NewMenu("File",
		saveItem,
		loadItem,
		fyne.NewMenuItemSeparator(),
		connectItem,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Clear History", func() {
			w.handleClearHistory()
		}),
	)

	// Edit menu - clear operations
	clearResponseItem := fyne.NewMenuItem("Clear Response", func() {
		w.handleClearResponse()
	})
	clearResponseItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyL,
		Modifier: fyne.KeyModifierSuper,
	}

	editMenu := fyne.NewMenu("Edit",
		fyne.NewMenuItem("Clear Request", func() {
			w.handleClearRequest()
		}),
		clearResponseItem,
	)

	// View menu - mode switching
	textModeItem := fyne.NewMenuItem("Text Mode", func() {
		w.requestPanel.SwitchToTextMode()
	})
	textModeItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.Key1,
		Modifier: fyne.KeyModifierSuper,
	}

	formModeItem := fyne.NewMenuItem("Form Mode", func() {
		w.requestPanel.SwitchToFormMode()
	})
	formModeItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.Key2,
		Modifier: fyne.KeyModifierSuper,
	}

	focusBrowserItem := fyne.NewMenuItem("Focus Service Browser", func() {
		w.serviceBrowser.FocusTree()
	})
	focusBrowserItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyB,
		Modifier: fyne.KeyModifierSuper,
	}

	filterServicesItem := fyne.NewMenuItem("Filter Services", func() {
		w.serviceBrowser.FocusFilter()
	})
	filterServicesItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyP,
		Modifier: fyne.KeyModifierSuper,
	}

	expandAllItem := fyne.NewMenuItem("Expand All Services", func() {
		w.serviceBrowser.ExpandAll()
	})
	expandAllItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyE,
		Modifier: fyne.KeyModifierSuper | fyne.KeyModifierShift,
	}

	collapseAllItem := fyne.NewMenuItem("Collapse All Services", func() {
		w.serviceBrowser.CollapseAll()
	})
	collapseAllItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyW,
		Modifier: fyne.KeyModifierSuper | fyne.KeyModifierShift,
	}

	viewMenu := fyne.NewMenu("View",
		textModeItem,
		formModeItem,
		fyne.NewMenuItemSeparator(),
		focusBrowserItem,
		filterServicesItem,
		expandAllItem,
		collapseAllItem,
	)

	// Help menu - shortcuts reference and about dialog
	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("Keyboard Shortcuts", func() {
			ShowShortcutDialog(w.window)
		}),
		fyne.NewMenuItemSeparator(),
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
	_ = w.state.Response.Size.Set("")
	w.responsePanel.ClearResponseMetadata()
	w.logger.Debug("response panel cleared")
}
