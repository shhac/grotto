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

