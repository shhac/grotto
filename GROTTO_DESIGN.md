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

### Key Fyne Concepts

```go
// Main window structure
app := app.New()
window := app.NewWindow("Grotto")

// Tabs for input modes
inputTabs := container.NewAppTabs(
    container.NewTabItem("Text", textEditor),
    container.NewTabItem("Form", formContainer),
)

// Form from proto fields (pseudo-code)
form := widget.NewForm(
    widget.NewFormItem("user_id", widget.NewEntry()),
    widget.NewFormItem("enabled", widget.NewCheck("", nil)),
    widget.NewFormItem("status", widget.NewSelect(enumValues, nil)),
)
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

    // 5. Use lenient parser (jhump/protoreflect)
    return desc.CreateFileDescriptorsFromSet(&descriptorpb.FileDescriptorSet{
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
    // gRPC reflection client
    "github.com/jhump/protoreflect/grpcreflect"

    // Lenient descriptor parsing
    "github.com/jhump/protoreflect/desc"

    // Dynamic message construction
    "github.com/jhump/protoreflect/dynamic"

    // Standard gRPC
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection/grpc_reflection_v1"
)
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
│       └── main.go           # Entry point
├── internal/
│   ├── ui/
│   │   ├── app.go            # Main window setup
│   │   ├── service_tree.go   # Service/method browser
│   │   ├── request_panel.go  # Input tabs (text/form)
│   │   ├── response_panel.go # Output tabs (text/form)
│   │   └── form_builder.go   # Proto → Fyne form generator
│   ├── reflection/
│   │   ├── client.go         # gRPC reflection client
│   │   ├── wellknown.go      # Well-known types bundle
│   │   ├── fixes.go          # Descriptor malformation fixes
│   │   └── resolver.go       # Type resolution strategies
│   ├── grpc/
│   │   ├── invoker.go        # Request execution
│   │   ├── streaming.go      # Stream handling
│   │   └── tls.go            # TLS configuration
│   └── storage/
│       ├── workspace.go      # Saved servers/requests
│       └── history.go        # Request history
├── go.mod
├── go.sum
└── README.md
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
    fyne.io/fyne/v2 v2.4+
    github.com/jhump/protoreflect v1.17+
    google.golang.org/grpc v1.60+
    google.golang.org/protobuf v1.32+
)
```

---

## Open Questions

1. **Embedded code editor?** Fyne's Entry is basic. Options:
   - Use Entry with custom styling (simplest)
   - Embed external editor component
   - Build syntax highlighting on top of Entry

2. **Streaming UI?** How to display:
   - Server streaming: Append messages to scrolling list
   - Client streaming: Queue of messages to send
   - Bidirectional: Split view with send/receive

3. **Large messages?** Proto messages can be huge:
   - Virtual scrolling for repeated fields
   - Lazy loading for nested messages
   - Collapse by default

4. **Persistence?** Options:
   - SQLite (robust but adds dependency)
   - JSON files (simple, human-readable)
   - Badger (Go-native KV store, used in Wombat)
