# Grotto

A permissive, user-friendly gRPC client in Go.

> **Name origin:** "Gr-" echoes "gRPC", and a grotto is a cozy cave for exploration — fitting for diving into APIs.

## Goals

1. **Permissive gRPC reflection** - Work with basically every server, handling malformed descriptors gracefully
2. **Dual input/output modes** - Raw text (grpcurl-style JSON) and structured forms (auto-generated from proto definitions)
3. **Native desktop UI** - Cross-platform, not web-based

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         Fyne GUI                                │
│  ┌──────────────────────┐    ┌─────────────────────────────┐   │
│  │   Service Browser    │    │      Request Panel          │   │
│  │   - Server list      │    │   ┌─────────┬─────────┐     │   │
│  │   - Service tree     │    │   │  Text   │  Form   │     │   │
│  │   - Method list      │    │   │  Mode   │  Mode   │     │   │
│  └──────────────────────┘    │   └─────────┴─────────┘     │   │
│                              └─────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Response Panel                         │  │
│  │              ┌─────────┬─────────┐                        │  │
│  │              │  Text   │  Form   │                        │  │
│  │              │  Mode   │  Mode   │                        │  │
│  │              └─────────┴─────────┘                        │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Core Engine                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌────────────────┐  │
│  │   Reflection    │  │   Proto Form    │  │    gRPC        │  │
│  │   Client        │  │   Generator     │  │    Invoker     │  │
│  │                 │  │                 │  │                │  │
│  │  - Lenient      │  │  - Field→Widget │  │  - Unary       │  │
│  │    parsing      │  │  - Validation   │  │  - Streaming   │  │
│  │  - Well-known   │  │  - Defaults     │  │  - Metadata    │  │
│  │    types        │  │                 │  │  - TLS         │  │
│  └─────────────────┘  └─────────────────┘  └────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

---

## UI Framework: Fyne

### Why Fyne?

| Criteria | Fyne | Gio | Qt/GTK |
|----------|------|-----|--------|
| Pure Go | ✅ | ✅ | ❌ (C bindings) |
| Form controls | ✅ Excellent | ⚠️ Manual | ✅ |
| Cross-platform | ✅ | ✅ | ✅ |
| Learning curve | Low | High (immediate-mode) | Medium |
| Maintenance | Active | Active | Qt bindings abandoned |
| Production ready | ✅ | ⚠️ Newer | ✅ |

**Fyne** is the recommended choice for a forms-heavy application:
- Built-in widgets: Entry, Select, Check, TextArea, Tree, List, Tabs
- Layout system designed for data entry forms
- Good documentation and examples
- GPU-accelerated rendering
- No C compiler required for end users

### Installation

```bash
go get fyne.io/fyne/v2@latest
```

### Architecture: Modified MVC with Data Binding

Fyne doesn't enforce an architecture, but for Grotto's complexity use centralized state with data binding:

**Model (Centralized State)**
```go
// internal/model/state.go
type ApplicationState struct {
    // Connection
    currentServer   binding.String
    connected       binding.Bool

    // Selection
    selectedService binding.String
    selectedMethod  binding.String

    // Request/Response
    request         *RequestState
    response        *ResponseState
}

type RequestState struct {
    mode      binding.String  // "text" or "form"
    textData  binding.String  // JSON
    errors    binding.StringList
    dirty     binding.Bool
}

type ResponseState struct {
    mode      binding.String
    textData  binding.String
    loading   binding.Bool
    streaming binding.Bool
    messages  binding.UntypedList  // For streaming responses
}
```

**View (Thin UI Layer)**
```go
// internal/ui/request/panel.go
type RequestPanel struct {
    widget.BaseWidget

    state      *model.RequestState
    textEditor *widget.Entry
}

func NewRequestPanel(state *model.RequestState) *RequestPanel {
    p := &RequestPanel{state: state}
    p.ExtendBaseWidget(p)

    // Bind UI to state - automatic updates
    p.textEditor = widget.NewEntryWithData(state.textData)

    // Listen to mode changes
    state.mode.AddListener(binding.NewDataListener(func() {
        mode, _ := state.mode.Get()
        p.switchMode(mode)
    }))

    return p
}
```

### Threading Model (Critical)

**Rule: All gRPC calls must run in goroutines. UI updates must use `fyne.Do()`.**

```go
// CORRECT: gRPC in goroutine, UI update via fyne.Do()
func (c *Controller) InvokeMethod(ctx context.Context) {
    c.state.response.loading.Set(true)

    go func() {
        resp, err := c.invoker.Invoke(ctx, method, request)

        fyne.Do(func() {  // Safe UI update from goroutine
            c.state.response.loading.Set(false)
            if err != nil {
                c.state.response.textData.Set(err.Error())
                return
            }
            c.state.response.textData.Set(protojson.Format(resp))
        })
    }()
}

// For streaming, append messages as they arrive
fyne.Do(func() {
    current, _ := c.state.response.messages.Get()
    c.state.response.messages.Set(append(current, msg))
})
```

### Custom Widget Pattern

```go
type CustomWidget struct {
    widget.BaseWidget  // MUST embed this

    // Widget state
    data string
}

func NewCustomWidget() *CustomWidget {
    w := &CustomWidget{}
    w.ExtendBaseWidget(w)  // CRITICAL: Register widget
    return w
}

func (w *CustomWidget) CreateRenderer() fyne.WidgetRenderer {
    // Return renderer that handles layout and drawing
}
```

### Testing Fyne Applications

```go
func TestWidget(t *testing.T) {
    app := test.NewApp()  // Headless test app
    defer app.Quit()

    w := NewCustomWidget()
    window := test.NewWindow(w)
    defer window.Close()

    test.Type(entry, "test input")  // Simulate user interaction
    assert.Equal(t, expected, widget.value)
}
```

---

## Permissive gRPC Reflection

### Core Strategy

The key insight from Wombat's development: servers return broken descriptors, and we must fix them.

```go
// Processing pipeline for server descriptors
func processDescriptors(fdset *descriptorpb.FileDescriptorSet) (*protoregistry.Files, error) {
    // 1. Pre-load all well-known types
    wellKnown := loadWellKnownTypes()

    // 2. Filter server files (remove conflicting google.protobuf)
    filtered := filterConflictingFiles(fdset.File)

    // 3. Fix common malformations
    for i, fd := range filtered {
        filtered[i] = fixReservedRanges(fd)
        filtered[i] = fixMapEntryNames(filtered[i])
        filtered[i] = injectMissingImports(filtered[i])
    }

    // 4. Build complete set with well-known types
    complete := append(wellKnown, filtered...)

    // 5. Use lenient parser (jhump/protoreflect/v2)
    return protodesc.NewFiles(&descriptorpb.FileDescriptorSet{
        File: complete,
    })
}
```

### Well-Known Types Bundle

Always include these 11 proto files from `protoregistry.GlobalFiles`:

```go
import (
    _ "google.golang.org/protobuf/types/known/anypb"
    _ "google.golang.org/protobuf/types/known/apipb"
    _ "google.golang.org/protobuf/types/known/durationpb"
    _ "google.golang.org/protobuf/types/known/emptypb"
    _ "google.golang.org/protobuf/types/known/fieldmaskpb"
    _ "google.golang.org/protobuf/types/known/sourcecontextpb"
    _ "google.golang.org/protobuf/types/known/structpb"
    _ "google.golang.org/protobuf/types/known/timestamppb"
    _ "google.golang.org/protobuf/types/known/typepb"
    _ "google.golang.org/protobuf/types/known/wrapperspb"
    _ "google.golang.org/protobuf/types/descriptorpb"
)

func loadWellKnownTypes() []*descriptorpb.FileDescriptorProto {
    var protos []*descriptorpb.FileDescriptorProto
    protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
        if strings.HasPrefix(string(fd.Path()), "google/protobuf/") {
            protos = append(protos, protodesc.ToFileDescriptorProto(fd))
        }
        return true
    })
    return protos
}
```

### Common Server Quirks to Handle

| Issue | Fix |
|-------|-----|
| Missing imports for well-known types | Auto-inject based on type references |
| Invalid reserved ranges (start > end) | Swap start/end values |
| Malformed map entry names | Rename to `<FieldName>Entry` pattern |
| Incomplete dependency tree | Iteratively fetch missing deps |
| v1alpha vs v1 reflection | Try v1 first, fallback to v1alpha |
| Conflicting google.protobuf files | Filter server versions, use ours |

### Recommended Libraries

```go
import (
    // gRPC reflection client (auto-detects v1/v1alpha)
    "github.com/jhump/protoreflect/v2/grpcreflect"

    // Dynamic gRPC invocation (unified API for all RPC types)
    "github.com/jhump/protoreflect/v2/grpcdynamic"

    // Dynamic message construction (stdlib)
    "google.golang.org/protobuf/types/dynamicpb"

    // JSON marshaling for text mode (stdlib)
    "google.golang.org/protobuf/encoding/protojson"

    // Descriptor types (stdlib)
    "google.golang.org/protobuf/reflect/protoreflect"

    // Standard gRPC
    "google.golang.org/grpc"
    "google.golang.org/grpc/keepalive"
    "google.golang.org/grpc/metadata"
)
```

### Reflection Client Setup (Critical Pattern)

```go
// Auto-detects v1 or v1alpha, with graceful fallback
refClient := grpcreflect.NewClientAuto(ctx, conn)
defer refClient.Reset()

// Configure for permissive operation (critical for broken servers)
refClient.AllowFallbackResolver(
    protoregistry.GlobalFiles,  // Fall back to local well-known types
    protoregistry.GlobalTypes,  // Extension types
)
refClient.AllowMissingFileDescriptors() // Build descriptors even with missing imports
```

### Connection Management

```go
// Use NewClient (not deprecated Dial)
conn, err := grpc.NewClient(
    addr,
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:                10 * time.Second, // Ping every 10s (GUI apps need this)
        Timeout:             3 * time.Second,
        PermitWithoutStream: true,             // Keep alive even when idle
    }),
)
```

### Dynamic Invocation

```go
// Unified stub for all RPC types
stub := grpcdynamic.NewStub(conn)

// Unary
response, err := stub.InvokeRpc(ctx, method, request)

// Server streaming
stream, err := stub.InvokeRpcServerStream(ctx, method, request)

// Client streaming
stream, err := stub.InvokeRpcClientStream(ctx, method)

// Bidirectional
stream, err := stub.InvokeRpcBidiStream(ctx, method)
```

---

## Input/Output Modes

### Text Mode

JSON editor for raw grpcurl-style input:

```json
{
  "user_id": "123",
  "request": {
    "page_size": 10,
    "filter": {
      "status": "ACTIVE"
    }
  }
}
```

**Implementation:**
- Use a syntax-highlighted text widget (or embed Monaco-like editor)
- Validate JSON on change
- Show proto validation errors inline

### Form Mode

Auto-generated UI from proto definitions:

```
┌─────────────────────────────────────────┐
│ user_id          [_________________]    │
│ request.page_size    [10        ▼]      │
│ request.filter.status                   │
│   ○ UNKNOWN  ● ACTIVE  ○ INACTIVE       │
└─────────────────────────────────────────┘
```

**Field Type → Widget Mapping:**

| Proto Type | Fyne Widget |
|------------|-------------|
| `string` | `Entry` |
| `int32/int64/uint*` | `Entry` with validation |
| `float/double` | `Entry` with validation |
| `bool` | `Check` |
| `enum` | `Select` or `RadioGroup` |
| `bytes` | `Entry` with base64 toggle |
| `message` | Nested form / expandable section |
| `repeated` | List with add/remove buttons |
| `map` | Key-value list with add/remove |
| `oneof` | `Select` + conditional fields |
| `google.protobuf.Timestamp` | Date/time picker |
| `google.protobuf.Duration` | Duration input (e.g., "5m30s") |

### Bidirectional Sync

Changes in one mode should reflect in the other:

```go
type RequestState struct {
    proto   protoreflect.Message
    json    string
    dirty   bool
}

// Text → Form
func (s *RequestState) UpdateFromJSON(json string) error {
    msg := dynamicpb.NewMessage(s.proto.Descriptor())
    if err := protojson.Unmarshal([]byte(json), msg); err != nil {
        return err
    }
    s.proto = msg
    s.json = json
    return nil
}

// Form → Text
func (s *RequestState) UpdateField(path string, value any) {
    setFieldByPath(s.proto, path, value)
    s.json = protojson.Format(s.proto)
}
```

---

## Project Structure

```
grotto/
├── cmd/
│   └── grotto/
│       └── main.go              # Minimal entry point
├── internal/
│   ├── app/                     # Application bootstrap & DI
│   │   ├── app.go               # Application coordination, wiring
│   │   └── config.go            # Configuration (env vars, config file)
│   ├── domain/                  # Core business logic (UI-independent)
│   │   ├── service.go           # Service/method models
│   │   ├── request.go           # Request state management
│   │   ├── workspace.go         # Workspace model
│   │   └── history.go           # History model
│   ├── model/                   # Centralized state with Fyne bindings
│   │   └── state.go             # ApplicationState with binding types
│   ├── ui/                      # Fyne UI layer (thin)
│   │   ├── window.go            # Main window setup
│   │   ├── browser/             # Service browser components
│   │   │   └── tree.go          # ServiceTreeWidget
│   │   ├── request/             # Request panel components
│   │   │   ├── panel.go         # Input tabs (text/form)
│   │   │   └── form_builder.go  # Proto → Fyne form generator
│   │   ├── response/            # Response panel components
│   │   │   ├── panel.go         # Output tabs (text/form)
│   │   │   └── streaming.go     # StreamingMessagesWidget
│   │   └── errors.go            # Error dialogs, status bar
│   ├── reflection/              # gRPC reflection client
│   │   ├── client.go            # Reflection wrapper with fallback
│   │   ├── wellknown.go         # Well-known types bundle
│   │   ├── fixes.go             # Descriptor malformation fixes
│   │   └── resolver.go          # Type resolution strategies
│   ├── grpc/                    # gRPC client operations
│   │   ├── connection.go        # Connection manager with state monitoring
│   │   ├── invoker.go           # Request executor (unary/streaming)
│   │   ├── handler.go           # InvocationHandler interface
│   │   └── metadata.go          # Header management
│   ├── errors/                  # Domain-specific error types
│   │   ├── errors.go            # Sentinel & typed errors
│   │   └── classify.go          # Error classification for UI
│   ├── logging/                 # Structured logging setup
│   │   └── logger.go            # slog configuration, platform paths
│   └── storage/                 # Persistence layer
│       ├── repository.go        # Storage interface
│       ├── json.go              # JSON file implementation
│       └── memory.go            # In-memory (for tests)
├── testdata/                    # Test fixtures (Go convention)
│   └── protos/
│       └── test.proto
├── go.mod
├── go.sum
└── README.md
```

### Key Structural Decisions

1. **`internal/app/`** - Application bootstrap and dependency injection. All components wired here via constructor injection.

2. **`internal/domain/`** - UI-independent business logic. Can be tested without Fyne.

3. **`internal/model/`** - Centralized state using Fyne's `binding` package for reactive UI updates.

4. **Split `internal/ui/`** - Organized by feature area (browser, request, response) to prevent monolithic package.

5. **`internal/errors/`** - Domain-specific error types with severity classification for proper UI presentation.

6. **`internal/logging/`** - Structured logging with `slog` (Go 1.21+ stdlib), platform-specific log file paths.

7. **`testdata/`** - Standard Go convention for test fixtures.

---

## Error Handling & Logging

### Error Strategy

Classify errors by severity for proper UI presentation:

```go
// internal/errors/errors.go
var (
    ErrConnectionFailed      = errors.New("connection failed")
    ErrReflectionUnavailable = errors.New("reflection not available")
    ErrInvalidDescriptor     = errors.New("invalid descriptor")
    ErrUserCancelled         = errors.New("user cancelled operation")
)

type ValidationError struct {
    Field   string
    Message string
}

// internal/errors/classify.go
type ErrorSeverity int

const (
    SeverityInfo    ErrorSeverity = iota // User should know, not blocking
    SeverityWarning                      // Degraded functionality
    SeverityError                        // Operation failed, can retry
    SeverityFatal                        // Application must exit
)

type UIError struct {
    Err      error
    Severity ErrorSeverity
    Title    string          // Short user-facing title
    Message  string          // Detailed user-facing message
    Recovery []string        // Suggested actions (bullet points)
    Actions  []ErrorAction   // Buttons for user actions
    Details  string          // Technical details (collapsed by default)
}

type ErrorAction struct {
    Label   string
    Handler func()
}

func ClassifyError(err error) *UIError {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        return &UIError{
            Severity: SeverityError,
            Title:    "Request Timeout",
            Message:  "The server took too long to respond.",
            Recovery: []string{"Try again", "Increase the timeout setting"},
            Actions:  []ErrorAction{{Label: "Retry"}, {Label: "Settings"}},
        }
    case errors.Is(err, ErrReflectionUnavailable):
        return &UIError{
            Severity: SeverityWarning,
            Title:    "Reflection Not Available",
            Message:  "This server doesn't support gRPC reflection.",
            Recovery: []string{"Import proto files manually"},
            Actions:  []ErrorAction{{Label: "Import Proto Files"}},
        }
    // ... more cases (see ClassifyGRPCError in UX section for gRPC codes)
    }
}
```

### Logging with slog (Go 1.21+)

```go
// internal/logging/logger.go
func InitLogger(appName string, debug bool) (*slog.Logger, error) {
    logPath, err := getLogFilePath(appName)  // Platform-specific
    if err != nil {
        return nil, err
    }

    logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        return nil, err
    }

    level := slog.LevelInfo
    if debug {
        level = slog.LevelDebug
    }

    return slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
        Level:     level,
        AddSource: debug,
    })), nil
}

// Platform-specific log paths:
// - macOS:   ~/Library/Logs/grotto/grotto.log
// - Linux:   ~/.local/state/grotto/grotto.log
// - Windows: %LOCALAPPDATA%\grotto\Logs\grotto.log
```

### Panic Recovery

```go
// cmd/grotto/main.go
func runApp(logger *slog.Logger) (err error) {
    defer func() {
        if r := recover(); r != nil {
            logger.Error("panic recovered",
                slog.Any("panic", r),
                slog.String("stack", string(debug.Stack())),
            )
            err = fmt.Errorf("panic: %v", r)
        }
    }()

    app := ui.NewApp(logger)
    app.Run()
    return nil
}

// For goroutines (async operations)
go func() {
    defer func() {
        if r := recover(); r != nil {
            logger.Error("panic in async invoke", slog.Any("panic", r))
            respChan <- &Response{Error: fmt.Errorf("internal error: %v", r)}
        }
    }()
    // ... work
}()
```

### Graceful Degradation

| Scenario | Behavior |
|----------|----------|
| Malformed descriptors | Fix and continue, log warning |
| Network timeout | Retry with backoff, show status |
| Reflection v1 fails | Fall back to v1alpha |
| Validation error | Show in UI, allow correction |
| User cancellation | Clean abort, restore state |

---

## User Experience (UX)

### Theming: Light/Dark Mode

Grotto supports three theme modes: System Default (follows OS preference), Light, and Dark.

#### The `forcedVariant` Pattern

Fyne's `theme.LightTheme()` and `theme.DarkTheme()` are deprecated. Use a wrapper that forces the variant:

```go
// internal/ui/theme.go
type forcedVariant struct {
    fyne.Theme
    variant fyne.ThemeVariant
}

func (f *forcedVariant) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
    return f.Theme.Color(name, f.variant)
}

func ApplyTheme(a fyne.App, mode string) {
    switch mode {
    case "dark":
        a.Settings().SetTheme(&forcedVariant{
            Theme:   theme.DefaultTheme(),
            variant: theme.VariantDark,
        })
    case "light":
        a.Settings().SetTheme(&forcedVariant{
            Theme:   theme.DefaultTheme(),
            variant: theme.VariantLight,
        })
    default: // "system"
        a.Settings().SetTheme(theme.DefaultTheme())
    }
}
```

#### Theme Persistence

Use Fyne's Preferences API to save/restore theme choice:

```go
const themePreferenceKey = "appTheme"

func LoadThemePreference(a fyne.App) {
    mode := a.Preferences().StringWithFallback(themePreferenceKey, "system")
    ApplyTheme(a, mode)
}

func SaveThemePreference(a fyne.App, mode string) {
    a.Preferences().SetString(themePreferenceKey, mode)
    ApplyTheme(a, mode)
}
```

#### Theme Selector UI

```go
func CreateThemeSelector(a fyne.App) *widget.Select {
    selector := widget.NewSelect(
        []string{"System Default", "Light", "Dark"},
        func(selected string) {
            var mode string
            switch selected {
            case "Dark":
                mode = "dark"
            case "Light":
                mode = "light"
            default:
                mode = "system"
            }
            SaveThemePreference(a, mode)
        },
    )

    // Set initial selection from preferences
    saved := a.Preferences().StringWithFallback(themePreferenceKey, "system")
    switch saved {
    case "dark":
        selector.SetSelected("Dark")
    case "light":
        selector.SetSelected("Light")
    default:
        selector.SetSelected("System Default")
    }
    return selector
}
```

#### Platform Auto-Detection

Fyne automatically monitors system theme changes:
- **macOS**: `AppleInterfaceThemeChangedNotification`
- **Windows**: Registry-based detection
- **Linux**: Freedesktop Portal D-Bus signals (Fyne v2.3+)

Environment override: `FYNE_THEME=light ./grotto` or `FYNE_THEME=dark ./grotto`

---

### Error Communication

#### Error Display Strategy

| Error Source | Severity | Display Method | Blocking? | Example |
|--------------|----------|----------------|-----------|---------|
| Connection refused | Error | Modal dialog | Yes | Server not reachable |
| Request timeout | Error | Modal with retry | Yes | Server took too long |
| Auth failure | Error | Modal with action | Yes | Credentials required |
| Reflection unavailable | Warning | Inline banner | No | Fall back to manual proto |
| Invalid field value | Error | Inline next to field | No | Number out of range |
| Successful invoke | Success | Toast notification | No | Request completed |
| Retry attempt | Info | Status bar | No | Retrying (2/3)... |
| Auto-fixed descriptor | Info | Log only | No | No user action needed |

#### gRPC Status Code Translation

Map gRPC codes to user-friendly messages:

```go
// internal/errors/grpc.go
func ClassifyGRPCError(err error) *UIError {
    st, ok := status.FromError(err)
    if !ok {
        return &UIError{
            Severity: SeverityError,
            Title:    "Unexpected Error",
            Message:  err.Error(),
            Recovery: []string{"Try again"},
        }
    }

    switch st.Code() {
    case codes.Unavailable:
        return &UIError{
            Severity: SeverityError,
            Title:    "Cannot Connect to Server",
            Message:  "The server is not responding.",
            Recovery: []string{
                "Check that the server is running",
                "Verify the address and port",
                "Check your network connection",
            },
            Actions:  []ErrorAction{{Label: "Retry"}, {Label: "Edit Connection"}},
            Details:  fmt.Sprintf("gRPC: %s - %s", st.Code(), st.Message()),
        }

    case codes.DeadlineExceeded:
        return &UIError{
            Severity: SeverityError,
            Title:    "Request Timeout",
            Message:  "The server took too long to respond.",
            Recovery: []string{"Try again", "Increase timeout setting"},
            Actions:  []ErrorAction{{Label: "Retry"}, {Label: "Settings"}},
        }

    case codes.Unauthenticated:
        return &UIError{
            Severity: SeverityError,
            Title:    "Authentication Required",
            Message:  "You need to authenticate to access this service.",
            Recovery: []string{"Add credentials in metadata"},
            Actions:  []ErrorAction{{Label: "Add Credentials"}},
        }

    case codes.PermissionDenied:
        return &UIError{
            Severity: SeverityError,
            Title:    "Access Denied",
            Message:  "You don't have permission to call this method.",
            Recovery: []string{"Contact administrator"},
        }

    case codes.InvalidArgument:
        return &UIError{
            Severity: SeverityError,
            Title:    "Invalid Request",
            Message:  "The request contains invalid data.",
            Recovery: []string{"Check field values", "See details for specifics"},
            Actions:  []ErrorAction{{Label: "View Details"}, {Label: "Edit Request"}},
            Details:  st.Message(),
        }

    case codes.Internal:
        return &UIError{
            Severity: SeverityError,
            Title:    "Server Error",
            Message:  "The server encountered an unexpected error.",
            Recovery: []string{"Try again later", "Contact server administrator"},
            Actions:  []ErrorAction{{Label: "Retry"}},
        }

    case codes.Unimplemented:
        return &UIError{
            Severity: SeverityWarning,
            Title:    "Method Not Available",
            Message:  "This method is not implemented on the server.",
            Recovery: []string{"Check method name", "Verify server version"},
        }

    default:
        return &UIError{
            Severity: SeverityError,
            Title:    "Request Failed",
            Message:  st.Message(),
            Recovery: []string{"Try again"},
            Details:  fmt.Sprintf("gRPC: %s", st.Code()),
        }
    }
}
```

#### Error Dialog Pattern

```go
// internal/ui/errors/display.go
func ShowGRPCError(err error, window fyne.Window, onRetry func()) {
    grpcErr := errors.ClassifyGRPCError(err)

    // Build content
    content := container.NewVBox(
        widget.NewLabel(grpcErr.Message),
    )

    // Add recovery suggestions
    if len(grpcErr.Recovery) > 0 {
        content.Add(widget.NewSeparator())
        content.Add(widget.NewLabel("You can:"))
        for _, suggestion := range grpcErr.Recovery {
            content.Add(widget.NewLabel("• " + suggestion))
        }
    }

    // Add expandable technical details
    if grpcErr.Details != "" {
        accordion := widget.NewAccordion(
            widget.NewAccordionItem("Technical Details",
                widget.NewLabel(grpcErr.Details)),
        )
        content.Add(accordion)
    }

    d := dialog.NewCustom(grpcErr.Title, "Close", content, window)
    d.Show()
}
```

#### Inline Validation Errors

For form fields, show errors inline without dialogs:

```go
// Show red text below problematic field
type ValidatedEntry struct {
    widget.Entry
    errorLabel *widget.Label
}

func (e *ValidatedEntry) SetError(msg string) {
    e.errorLabel.SetText(msg)
    e.errorLabel.Show()
}

func (e *ValidatedEntry) ClearError() {
    e.errorLabel.Hide()
}
```

#### Connection State Indicator

Display connection health in status bar:

```go
type ConnectionState int

const (
    StateDisconnected ConnectionState = iota  // Gray
    StateConnecting                           // Yellow/Amber
    StateConnected                            // Green
    StateError                                // Red
)

// Status bar shows: "● Connected to api.example.com:50051"
```

---

### Auto-Retry with Backoff

Automatically retry transient errors:

```go
// internal/grpc/retry.go
type RetryConfig struct {
    MaxAttempts       int
    InitialBackoff    time.Duration
    MaxBackoff        time.Duration
    BackoffMultiplier float64
}

var DefaultRetryConfig = RetryConfig{
    MaxAttempts:       3,
    InitialBackoff:    100 * time.Millisecond,
    MaxBackoff:        5 * time.Second,
    BackoffMultiplier: 2.0,
}

// Only retry transient errors
func isRetryable(err error) bool {
    st, ok := status.FromError(err)
    if !ok {
        return false
    }
    switch st.Code() {
    case codes.Unavailable, codes.DeadlineExceeded,
         codes.ResourceExhausted, codes.Internal:
        return true
    }
    return false
}
```

**UX during retry:**
- Show "Retrying... (2/3)" in status bar
- Allow user to cancel
- On success after retry: brief toast "Connected after 2 attempts"
- On max retries: show modal with "Retry Manually" option

---

### Form State Preservation

**Critical UX rule: Never clear user input on error.**

```go
type FormState struct {
    fields map[string]interface{}
    errors map[string]string
}

func (f *FormState) OnError(err error) {
    // Show error, but preserve all field values
    // Only clear error messages when user edits the field
}
```

---

### Error Message Best Practices

Based on platform guidelines (Apple HIG, Windows UX):

1. **Be specific** - "Cannot connect to api.example.com" not "Connection error"
2. **Explain why** - "The server is not responding" not just "Failed"
3. **Offer recovery** - Actionable steps the user can take
4. **Don't blame the user** - "Invalid email format" not "You entered wrong email"
5. **Technical details: hidden by default** - Expandable accordion for debugging
6. **One problem at a time** - Don't overwhelm with multiple issues

**Example Error Messages:**

```
❌ Poor: Error: Connection failed

✅ Good: Cannot Connect to Server
        The server at api.example.com:50051 is not responding.

        You can:
        • Check that the server is running
        • Verify the address and port
        • Check your network connection

        [Retry]  [Edit Connection]

        ▸ Technical Details
```

---

## Implementation Phases

### Phase 1: Core Reflection Engine
- [ ] Set up Go module with Fyne
- [ ] Port reflection client from Wombat
- [ ] Implement well-known types bundle
- [ ] Implement descriptor fixes (reserved ranges, map entries, missing imports)
- [ ] Add unit tests for edge cases

### Phase 2: Basic UI
- [ ] Main window with service browser
- [ ] Connect to server, list services/methods
- [ ] Text mode input (JSON editor)
- [ ] Text mode output (JSON display)
- [ ] Basic unary RPC execution

### Phase 3: Form Mode
- [ ] Proto field → widget mapper
- [ ] Form builder for messages
- [ ] Handle nested messages
- [ ] Handle repeated fields
- [ ] Handle oneofs
- [ ] Bidirectional text ↔ form sync

### Phase 4: Advanced Features
- [ ] Streaming RPCs (server, client, bidirectional)
- [ ] Metadata/headers management
- [ ] TLS configuration
- [ ] Workspaces (save servers/requests)
- [ ] Request history

### Phase 5: Polish
- [ ] Keyboard shortcuts
- [ ] Dark/light themes
- [ ] Error handling UX
- [ ] Performance optimization

---

## Key Dependencies

```go
require (
    fyne.io/fyne/v2 v2.4+               // GUI framework
    github.com/jhump/protoreflect/v2 v2.0.0-beta.2+ // grpcreflect, grpcdynamic
    google.golang.org/grpc v1.77+        // gRPC (use NewClient, not Dial)
    google.golang.org/protobuf v1.32+    // dynamicpb, protojson
)

// Note: Go 1.21+ required for slog (structured logging)
```

### Dependency Injection Pattern

Use constructor injection in `internal/app/app.go`:

```go
type App struct {
    config      *Config
    logger      *slog.Logger
    connManager *grpc.ConnectionManager
    storage     storage.Repository  // Interface, not concrete type
    window      *ui.Window
}

func New(cfg *Config) (*App, error) {
    logger, err := logging.InitLogger("grotto", cfg.Debug)
    if err != nil {
        return nil, err
    }

    // Wire dependencies via constructors
    connManager := grpc.NewConnectionManager(logger)
    storage := storage.NewJSONRepository(cfg.StoragePath)  // Implements Repository
    window := ui.NewWindow(connManager, storage, logger)

    return &App{
        config:      cfg,
        logger:      logger,
        connManager: connManager,
        storage:     storage,
        window:      window,
    }, nil
}
```

---

## Open Questions & Decisions

### Resolved Questions

1. **Streaming UI?** → **Use `binding.UntypedList` with auto-scrolling widget**
   - Server streaming: `StreamingMessagesWidget` that appends to list, auto-scrolls
   - Client streaming: Queue with send button, show "sent" count
   - Bidirectional: Split panel with separate send/receive lists

2. **Persistence?** → **JSON files (simple, human-readable)**
   - Storage interface in `internal/storage/repository.go`
   - JSON implementation for production
   - In-memory implementation for tests
   - Files stored in `~/.grotto/` (cross-platform via `os.UserHomeDir()`)

3. **Architecture?** → **Modified MVC with Fyne Data Binding**
   - Centralized state in `internal/model/` using `binding` package
   - Controllers for business logic, views are thin UI bindings
   - Automatic UI updates via binding listeners

4. **Threading?** → **goroutines + `fyne.Do()`**
   - All gRPC calls in goroutines
   - UI updates via `fyne.Do()` (fire-and-forget) or `fyne.DoAndWait()` (sync)
   - Context for user cancellation

5. **Logging?** → **slog (Go 1.21+ stdlib)**
   - Platform-specific log file paths
   - JSON format for structured logs
   - DEBUG level in development, INFO in production

### Open Questions

1. **Embedded code editor?** Fyne's Entry is basic. Options:
   - Use multiline Entry with custom styling (simplest, start here)
   - Build syntax highlighting later if needed
   - Consider `fyne.io/x/fyne` extensions

2. **Large messages?** Proto messages can be huge:
   - Virtual scrolling for repeated fields (Fyne `List` supports this)
   - Lazy loading for nested messages
   - Collapse by default, expand on demand

3. **Offline proto files?** For servers without reflection:
   - Parse `.proto` files directly
   - Support compiled `.protoset` files (like grpcurl)
   - Lower priority (Phase 5+)
