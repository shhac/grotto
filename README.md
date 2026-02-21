# Grotto

A permissive, user-friendly gRPC client with a native desktop GUI.

## Features

- **Reflection-based discovery** — Automatically discovers services and methods via gRPC Server Reflection, with permissive handling of malformed server descriptors
- **Dual interaction modes**:
  - **Form mode** — Auto-generated forms with validation, nested message support, maps, repeated fields, and oneofs
  - **Text mode** — Direct JSON editing with bidirectional sync to form mode
- **Streaming support** — Unary, server streaming, client streaming, and bidirectional streaming RPCs
- **Well-known types** — Native form widgets for Timestamp (RFC3339), Duration, and FieldMask fields
- **Metadata** — Send and inspect gRPC request/response metadata headers
- **TLS support** — Secure connections with configurable TLS, mTLS, and skip-verify options
- **Workspaces** — Save and load connections, selected methods, and request data
- **Request history** — Track and replay previous requests
- **Keyboard shortcuts** — See [SHORTCUTS.md](SHORTCUTS.md) for the full list

## Install

### Homebrew (macOS)

```bash
brew install shhac/tap/grotto
```

### Build from source

Requires Go 1.25+ and [Fyne system dependencies](https://developer.fyne.io/started/).

```bash
go build -o grotto ./cmd/grotto
./grotto
```

## Development

### Test Servers

The project includes several test gRPC servers for development and testing. See [testdata/README.md](testdata/README.md) for details.

## License

MIT
