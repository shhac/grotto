package grpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Invoker handles dynamic gRPC invocations using reflection-based message types.
// It supports unary and streaming RPC patterns without requiring generated code.
type Invoker struct {
	conn   *grpc.ClientConn
	logger *slog.Logger
	stub   grpcdynamic.Stub
}

// NewInvoker creates a new dynamic gRPC invoker for the given connection.
func NewInvoker(conn *grpc.ClientConn, logger *slog.Logger) *Invoker {
	return &Invoker{
		conn:   conn,
		logger: logger,
		stub:   grpcdynamic.NewStub(conn),
	}
}

// InvokeUnary calls a unary RPC method dynamically.
//
// Parameters:
//   - methodDesc: Method descriptor from reflection client
//   - jsonRequest: JSON string representation of the request message
//   - md: gRPC metadata (headers) to send with the request
//
// Returns:
//   - jsonResponse: JSON string representation of the response message
//   - responseHeaders: gRPC metadata (headers) received from the server
//   - err: Error if invocation fails or JSON marshaling fails
func (i *Invoker) InvokeUnary(
	ctx context.Context,
	methodDesc *desc.MethodDescriptor,
	jsonRequest string,
	md metadata.MD,
) (jsonResponse string, responseHeaders metadata.MD, err error) {
	methodName := methodDesc.GetFullyQualifiedName()
	i.logger.Debug("invoking unary RPC",
		slog.String("method", methodName),
		slog.String("request", jsonRequest),
	)

	// Create dynamic request message from method descriptor
	reqMsg := dynamic.NewMessage(methodDesc.GetInputType())

	// Unmarshal JSON into dynamic message
	if err := reqMsg.UnmarshalJSON([]byte(jsonRequest)); err != nil {
		i.logger.Error("failed to unmarshal request JSON",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return "", nil, fmt.Errorf("invalid request JSON: %w", err)
	}

	// Prepare call options to capture response headers
	var respHeaders metadata.MD
	callOpts := []grpc.CallOption{
		grpc.Header(&respHeaders),
	}

	// Add request metadata if provided
	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// Invoke the RPC using dynamic stub
	respMsg, err := i.stub.InvokeRpc(ctx, methodDesc, reqMsg, callOpts...)
	if err != nil {
		i.logger.Error("RPC invocation failed",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return "", respHeaders, err
	}

	// Marshal response to JSON
	jsonBytes, err := respMsg.(*dynamic.Message).MarshalJSON()
	if err != nil {
		i.logger.Error("failed to marshal response to JSON",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return "", respHeaders, fmt.Errorf("failed to format response: %w", err)
	}

	i.logger.Debug("unary RPC completed",
		slog.String("method", methodName),
		slog.String("response", string(jsonBytes)),
	)

	return string(jsonBytes), respHeaders, nil
}

// InvokeServerStream calls a server streaming RPC method dynamically.
//
// Parameters:
//   - methodDesc: Method descriptor from reflection client
//   - jsonRequest: JSON string representation of the request message
//   - md: gRPC metadata (headers) to send with the request
//
// Returns:
//   - msgChan: Channel that receives JSON-formatted response messages
//   - errChan: Channel that receives errors (including io.EOF when stream completes)
//
// The caller should read from both channels until errChan receives io.EOF (normal completion)
// or a non-EOF error (failure). The channels are closed when the stream ends.
func (i *Invoker) InvokeServerStream(
	ctx context.Context,
	methodDesc *desc.MethodDescriptor,
	jsonRequest string,
	md metadata.MD,
) (<-chan string, <-chan error) {
	msgChan := make(chan string, 10)  // Buffered to avoid blocking on send
	errChan := make(chan error, 1)

	methodName := methodDesc.GetFullyQualifiedName()
	i.logger.Debug("invoking server streaming RPC",
		slog.String("method", methodName),
		slog.String("request", jsonRequest),
	)

	go func() {
		defer close(msgChan)
		defer close(errChan)

		// Create dynamic request message
		reqMsg := dynamic.NewMessage(methodDesc.GetInputType())

		// Unmarshal JSON into dynamic message
		if err := reqMsg.UnmarshalJSON([]byte(jsonRequest)); err != nil {
			i.logger.Error("failed to unmarshal request JSON",
				slog.String("method", methodName),
				slog.Any("error", err),
			)
			errChan <- fmt.Errorf("invalid request JSON: %w", err)
			return
		}

		// Add request metadata if provided
		if len(md) > 0 {
			ctx = metadata.NewOutgoingContext(ctx, md)
		}

		// Invoke the server streaming RPC
		stream, err := i.stub.InvokeRpcServerStream(ctx, methodDesc, reqMsg)
		if err != nil {
			i.logger.Error("failed to start server stream",
				slog.String("method", methodName),
				slog.Any("error", err),
			)
			errChan <- err
			return
		}

		// Receive messages from stream
		messageCount := 0
		for {
			respMsg, err := stream.RecvMsg()
			if err == io.EOF {
				i.logger.Debug("server stream completed",
					slog.String("method", methodName),
					slog.Int("message_count", messageCount),
				)
				errChan <- io.EOF
				return
			}
			if err != nil {
				i.logger.Error("stream receive error",
					slog.String("method", methodName),
					slog.Int("message_count", messageCount),
					slog.Any("error", err),
				)
				errChan <- err
				return
			}

			// Marshal message to JSON
			jsonBytes, err := respMsg.(*dynamic.Message).MarshalJSON()
			if err != nil {
				i.logger.Error("failed to marshal stream message to JSON",
					slog.String("method", methodName),
					slog.Any("error", err),
				)
				errChan <- fmt.Errorf("failed to format stream message: %w", err)
				return
			}

			messageCount++
			i.logger.Debug("received stream message",
				slog.String("method", methodName),
				slog.Int("message_num", messageCount),
			)

			// Send JSON message to channel
			select {
			case msgChan <- string(jsonBytes):
			case <-ctx.Done():
				i.logger.Info("server stream cancelled by context",
					slog.String("method", methodName),
					slog.Int("message_count", messageCount),
				)
				errChan <- ctx.Err()
				return
			}
		}
	}()

	return msgChan, errChan
}

// ClientStreamHandle represents an active client streaming RPC session.
// It provides methods to send messages and close the stream to receive the final response.
type ClientStreamHandle struct {
	stream     *grpcdynamic.ClientStream
	methodDesc *desc.MethodDescriptor
	logger     *slog.Logger
}

// Send sends a JSON message on the client stream.
// Returns an error if the JSON is invalid or the send fails.
func (h *ClientStreamHandle) Send(jsonRequest string) error {
	methodName := h.methodDesc.GetFullyQualifiedName()
	h.logger.Debug("sending client stream message",
		slog.String("method", methodName),
		slog.String("request", jsonRequest),
	)

	// Create dynamic request message
	reqMsg := dynamic.NewMessage(h.methodDesc.GetInputType())

	// Unmarshal JSON into dynamic message
	if err := reqMsg.UnmarshalJSON([]byte(jsonRequest)); err != nil {
		h.logger.Error("failed to unmarshal request JSON",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return fmt.Errorf("invalid request JSON: %w", err)
	}

	// Send message on stream
	if err := h.stream.SendMsg(reqMsg); err != nil {
		h.logger.Error("failed to send client stream message",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return err
	}

	h.logger.Debug("client stream message sent",
		slog.String("method", methodName),
	)

	return nil
}

// CloseAndReceive closes the send side of the stream and receives the final response.
// Returns the JSON-formatted response or an error.
func (h *ClientStreamHandle) CloseAndReceive() (string, error) {
	methodName := h.methodDesc.GetFullyQualifiedName()
	h.logger.Debug("closing client stream and receiving response",
		slog.String("method", methodName),
	)

	// Close send side and receive final response
	respMsg, err := h.stream.CloseAndReceive()
	if err != nil {
		h.logger.Error("failed to close and receive client stream response",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return "", err
	}

	// Marshal response to JSON
	jsonBytes, err := respMsg.(*dynamic.Message).MarshalJSON()
	if err != nil {
		h.logger.Error("failed to marshal response to JSON",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return "", fmt.Errorf("failed to format response: %w", err)
	}

	h.logger.Debug("client stream completed",
		slog.String("method", methodName),
		slog.String("response", string(jsonBytes)),
	)

	return string(jsonBytes), nil
}

// InvokeClientStream starts a client streaming RPC and returns a handle for sending messages.
//
// Parameters:
//   - methodDesc: Method descriptor from reflection client
//   - md: gRPC metadata (headers) to send with the request
//
// Returns:
//   - handle: Handle for sending messages and receiving the final response
//   - err: Error if stream creation fails
//
// Usage:
//   handle, err := invoker.InvokeClientStream(ctx, methodDesc, md)
//   if err != nil { ... }
//
//   // Send multiple messages
//   handle.Send(`{"id": "1"}`)
//   handle.Send(`{"id": "2"}`)
//
//   // Close stream and get response
//   response, err := handle.CloseAndReceive()
func (i *Invoker) InvokeClientStream(
	ctx context.Context,
	methodDesc *desc.MethodDescriptor,
	md metadata.MD,
) (*ClientStreamHandle, error) {
	methodName := methodDesc.GetFullyQualifiedName()
	i.logger.Debug("invoking client streaming RPC",
		slog.String("method", methodName),
	)

	// Add request metadata if provided
	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// Invoke the client streaming RPC
	stream, err := i.stub.InvokeRpcClientStream(ctx, methodDesc)
	if err != nil {
		i.logger.Error("failed to start client stream",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return nil, err
	}

	i.logger.Debug("client stream started",
		slog.String("method", methodName),
	)

	return &ClientStreamHandle{
		stream:     stream,
		methodDesc: methodDesc,
		logger:     i.logger,
	}, nil
}

// BidiStreamHandle represents an active bidirectional streaming RPC session.
// It provides methods to send messages, receive messages, and close the send side.
type BidiStreamHandle struct {
	stream     *grpcdynamic.BidiStream
	methodDesc *desc.MethodDescriptor
	logger     *slog.Logger
}

// Send sends a JSON message on the bidirectional stream.
// Returns an error if the JSON is invalid or the send fails.
func (h *BidiStreamHandle) Send(jsonRequest string) error {
	methodName := h.methodDesc.GetFullyQualifiedName()
	h.logger.Debug("sending bidi stream message",
		slog.String("method", methodName),
		slog.String("request", jsonRequest),
	)

	// Create dynamic request message
	reqMsg := dynamic.NewMessage(h.methodDesc.GetInputType())

	// Unmarshal JSON into dynamic message
	if err := reqMsg.UnmarshalJSON([]byte(jsonRequest)); err != nil {
		h.logger.Error("failed to unmarshal request JSON",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return fmt.Errorf("invalid request JSON: %w", err)
	}

	// Send message on stream
	if err := h.stream.SendMsg(reqMsg); err != nil {
		h.logger.Error("failed to send bidi stream message",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return err
	}

	h.logger.Debug("bidi stream message sent",
		slog.String("method", methodName),
	)

	return nil
}

// Recv receives a JSON message from the bidirectional stream.
// Returns the JSON string, or io.EOF when the server closes the stream.
func (h *BidiStreamHandle) Recv() (string, error) {
	methodName := h.methodDesc.GetFullyQualifiedName()

	respMsg, err := h.stream.RecvMsg()
	if err == io.EOF {
		h.logger.Debug("bidi stream receive completed",
			slog.String("method", methodName),
		)
		return "", io.EOF
	}
	if err != nil {
		h.logger.Error("bidi stream receive error",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return "", err
	}

	// Marshal message to JSON
	jsonBytes, err := respMsg.(*dynamic.Message).MarshalJSON()
	if err != nil {
		h.logger.Error("failed to marshal bidi stream message to JSON",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return "", fmt.Errorf("failed to format stream message: %w", err)
	}

	h.logger.Debug("received bidi stream message",
		slog.String("method", methodName),
	)

	return string(jsonBytes), nil
}

// CloseSend closes the send side of the bidirectional stream.
// The client can still receive messages after closing the send side.
func (h *BidiStreamHandle) CloseSend() error {
	methodName := h.methodDesc.GetFullyQualifiedName()
	h.logger.Debug("closing bidi stream send side",
		slog.String("method", methodName),
	)

	if err := h.stream.CloseSend(); err != nil {
		h.logger.Error("failed to close bidi stream send side",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return err
	}

	h.logger.Debug("bidi stream send side closed",
		slog.String("method", methodName),
	)

	return nil
}

// InvokeBidiStream starts a bidirectional streaming RPC and returns a handle for sending and receiving messages.
//
// Parameters:
//   - methodDesc: Method descriptor from reflection client
//   - md: gRPC metadata (headers) to send with the request
//
// Returns:
//   - handle: Handle for sending and receiving messages
//   - err: Error if stream creation fails
//
// Usage:
//   handle, err := invoker.InvokeBidiStream(ctx, methodDesc, md)
//   if err != nil { ... }
//
//   // Start a goroutine to receive messages
//   go func() {
//       for {
//           msg, err := handle.Recv()
//           if err == io.EOF { return }
//           if err != nil { ... }
//           // Process msg
//       }
//   }()
//
//   // Send multiple messages
//   handle.Send(`{"id": "1"}`)
//   handle.Send(`{"id": "2"}`)
//
//   // Close send side when done
//   handle.CloseSend()
func (i *Invoker) InvokeBidiStream(
	ctx context.Context,
	methodDesc *desc.MethodDescriptor,
	md metadata.MD,
) (*BidiStreamHandle, error) {
	methodName := methodDesc.GetFullyQualifiedName()
	i.logger.Debug("invoking bidirectional streaming RPC",
		slog.String("method", methodName),
	)

	// Add request metadata if provided
	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// Invoke the bidirectional streaming RPC
	stream, err := i.stub.InvokeRpcBidiStream(ctx, methodDesc)
	if err != nil {
		i.logger.Error("failed to start bidi stream",
			slog.String("method", methodName),
			slog.Any("error", err),
		)
		return nil, err
	}

	i.logger.Debug("bidi stream started",
		slog.String("method", methodName),
	)

	return &BidiStreamHandle{
		stream:     stream,
		methodDesc: methodDesc,
		logger:     i.logger,
	}, nil
}

