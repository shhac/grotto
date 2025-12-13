# Test Data

This directory contains test gRPC servers for developing and testing Grotto functionality.

## Quick Start

All test servers have gRPC reflection enabled for easy testing with Grotto.

| Server | Port | Description | Run Command |
|--------|------|-------------|-------------|
| server | 50051 | Basic greeter with Health service | `cd server && go run main.go` |
| kitchensink | 50052 | All field types, nested, maps, oneofs | `cd kitchensink && go run main.go` |
| recursive | 50053 | Self-referencing types (tree, linked list) | `cd recursive && go run main.go` |
| bidistream | 50054 | Bidirectional streaming echo | `cd bidistream && go run main.go` |

## Test Servers

### server (port 50051)
Basic test server with a simple Greeter service and the standard gRPC Health service. Ideal for testing basic connection, reflection, and unary RPC functionality.

### kitchensink (port 50052)
Comprehensive test server featuring all protobuf field types, nested messages, maps, repeated fields, and oneofs. Use this to test Grotto's handling of complex message structures and type rendering.

### recursive (port 50053)
Tests self-referencing message types including tree structures and linked lists. Validates Grotto's ability to handle recursive type definitions without infinite loops.

### bidistream (port 50054)
Bidirectional streaming echo server. Tests Grotto's streaming capabilities where both client and server can send multiple messages over a single connection.

## Using with Grotto

1. Start any test server using the run command from the table above
2. Launch Grotto: `./grotto`
3. In the connection bar, enter the server's address (e.g., `localhost:50051`)
4. Click "Connect"
5. Browse the available services and test requests

Check individual server directories for detailed documentation about specific services and message types.
