# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Grotto is a permissive, user-friendly gRPC client written in Go with a Fyne-based native desktop UI. The name echoes "gRPC" while evoking a cozy cave for API exploration.

## Development Commands

```bash
# Install dependencies
go mod download

# Build the application
go build ./cmd/grotto

# Run the application
go run ./cmd/grotto

# Run tests
go test ./...

# Run a specific test
go test -run TestName ./internal/reflection/

# Format code
go fmt ./...

# Vet code
go vet ./...
```

## Architecture

### Core Components

1. **Reflection Client** (`internal/reflection/`) - Permissive gRPC reflection that handles malformed server descriptors
   - Pre-loads well-known types from `protoregistry.GlobalFiles`
   - Filters conflicting google.protobuf files from servers
   - Fixes common issues: invalid reserved ranges, malformed map entries, missing imports
   - Uses `github.com/jhump/protoreflect` for lenient parsing

2. **Proto Form Generator** (`internal/ui/form/`) - Converts proto field definitions to Fyne widgets
   - `mapper.go` maps proto types to appropriate widgets (Entry, Check, Select, etc.)
   - `builder.go` handles nested messages, repeated fields, oneofs, and maps
   - Special handling for well-known types (Timestamp, Duration, FieldMask)

3. **gRPC Invoker** (`internal/grpc/`) - Request execution supporting unary and streaming RPCs

4. **UI Layer** (`internal/ui/`) - Fyne-based interface with dual input/output modes (Text/Form)

### Key Design Decisions

- **Fyne over Gio/Qt**: Chosen for excellent form controls, pure Go (no C compiler needed), and low learning curve
- **jhump/protoreflect**: Used for lenient descriptor parsing and dynamic message construction
- **Bidirectional sync**: Text mode (JSON) and Form mode stay synchronized via shared `RequestState`

### Well-Known Types

The reflection client must bundle these 11 proto files from Google's protobuf library:
- anypb, apipb, durationpb, emptypb, fieldmaskpb
- sourcecontextpb, structpb, timestamppb, typepb, wrapperspb, descriptorpb

### Key Dependencies

- `fyne.io/fyne/v2` - UI framework
- `github.com/jhump/protoreflect` - Reflection client and lenient parsing
- `google.golang.org/grpc` - gRPC client
- `google.golang.org/protobuf` - Protobuf runtime
