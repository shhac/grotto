# Test Data

This directory contains test servers and data for verifying Grotto functionality.

## Test gRPC Server

Location: `testdata/server/`

The test server provides a simple gRPC health check service for testing Grotto's connection and reflection capabilities.

### Running the Test Server

```bash
cd testdata/server
./server
```

The server will:
- Listen on `localhost:50051`
- Expose the `grpc.health.v1.Health` service
- Enable gRPC reflection

### Using with Grotto

1. Start the test server (as shown above)
2. Launch Grotto: `./grotto`
3. In the connection bar, enter: `localhost:50051`
4. Click "Connect"
5. You should see the health service appear in the service browser

### Rebuilding the Server

If you make changes to the server code:

```bash
cd testdata/server
go build -o server main.go
```

### What's Provided

- **grpc.health.v1.Health service**: Standard gRPC health checking service
- **gRPC reflection**: Allows Grotto to discover services and methods dynamically

This is the minimal setup needed to test Grotto's core functionality without requiring custom proto files.
