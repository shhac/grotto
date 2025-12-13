# BidiStream Echo Test Server

A simple bidirectional streaming gRPC server for testing Grotto's bidi streaming UI.

## Proto Structure

The service has a single bidirectional streaming RPC:

```protobuf
service EchoService {
  rpc BidiEcho(stream PingRequest) returns (stream PongResponse);
}

message PingRequest {
  string ping = 1;
}

message PongResponse {
  string pong = 1;
}
```

## Behavior

- For each `PingRequest` received, the server immediately sends a `PongResponse` with `pong = ping`
- All messages are logged to console for debugging
- Server runs on port **50054**
- Reflection is enabled for tools like grpcurl

## How to Run

```bash
cd testdata/bidistream
go run server.go
```

The server will start on `localhost:50054`.

## Testing with grpcurl

List services:
```bash
grpcurl -plaintext localhost:50054 list
```

Invoke the bidirectional stream:
```bash
grpcurl -plaintext -d '{"ping":"hello"}' localhost:50054 echo.EchoService/BidiEcho
```
