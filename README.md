# Grotto

A permissive, user-friendly gRPC client with a modern GUI.

<!-- Screenshot placeholder - TODO: Add screenshot -->

## Features

- **Reflection-based discovery** - Automatically discovers services and methods via gRPC Server Reflection
- **Dual interaction modes**:
  - **Form mode** - User-friendly forms for request construction
  - **Text mode** - Direct JSON editing for advanced users
- **Streaming support** - Full support for unary, server streaming, client streaming, and bidirectional streaming
- **TLS support** - Secure connections with configurable TLS options
- **Workspaces** - Organize and persist your gRPC connections and requests
- **Request history** - Track and replay previous requests

## Requirements

- Go 1.21 or later
- Fyne dependencies (system libraries required for GUI)
  - See [Fyne documentation](https://developer.fyne.io/started/) for platform-specific requirements

## Build

```bash
go build -o grotto ./cmd/grotto
```

## Run

```bash
./grotto
```

## Keyboard Shortcuts

See [SHORTCUTS.md](SHORTCUTS.md) for a complete list of keyboard shortcuts and navigation tips.

## Development

### Test Servers

The project includes several test gRPC servers for development and testing purposes. See [testdata/README.md](testdata/README.md) for details on running them.

## License

MIT
