package reflection

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/shhac/grotto/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Client wraps gRPC reflection operations with permissive error handling.
// It auto-detects v1/v1alpha reflection protocols and gracefully handles
// malformed descriptors using fallback resolvers and fixes.
type Client struct {
	conn   *grpc.ClientConn
	logger *slog.Logger
}

// NewClient creates a new reflection client for the given connection.
func NewClient(conn *grpc.ClientConn, logger *slog.Logger) *Client {
	return &Client{
		conn:   conn,
		logger: logger,
	}
}

// ListServices returns all services discovered via gRPC reflection.
// It automatically detects v1 or v1alpha reflection protocols and applies
// permissive parsing with fallback to well-known types.
func (c *Client) ListServices(ctx context.Context) ([]domain.Service, error) {
	c.logger.Debug("listing services via reflection")

	// Create reflection client with auto-detection of v1/v1alpha
	refClient := grpcreflect.NewClientAuto(ctx, c.conn)
	defer refClient.Reset()

	// Configure for permissive operation (critical for broken servers)
	refClient.AllowFallbackResolver(
		protoregistry.GlobalFiles, // Fall back to local well-known types
		protoregistry.GlobalTypes,  // Extension types
	)
	refClient.AllowMissingFileDescriptors() // Build descriptors even with missing imports

	// List all services
	serviceNames, err := refClient.ListServices()
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Filter out grpc.reflection service (internal)
	var services []domain.Service
	for _, serviceName := range serviceNames {
		if serviceName == "grpc.reflection.v1alpha.ServerReflection" ||
			serviceName == "grpc.reflection.v1.ServerReflection" {
			c.logger.Debug("skipping internal reflection service", "service", serviceName)
			continue
		}

		c.logger.Debug("resolving service", "service", serviceName)
		serviceDesc, err := refClient.ResolveService(serviceName)
		if err != nil {
			c.logger.Warn("failed to resolve service", "service", serviceName, "error", err)
			continue
		}

		// Convert to domain.Service
		service := c.convertServiceDescriptor(serviceDesc)
		services = append(services, service)
	}

	c.logger.Info("discovered services", "count", len(services))
	return services, nil
}

// GetMethodDescriptor returns the descriptor for a specific method.
// The fullMethodName should be in the format "/package.Service/Method".
func (c *Client) GetMethodDescriptor(ctx context.Context, fullMethodName string) (protoreflect.MethodDescriptor, error) {
	c.logger.Debug("getting method descriptor", "method", fullMethodName)

	refClient := grpcreflect.NewClientAuto(ctx, c.conn)
	defer refClient.Reset()

	refClient.AllowFallbackResolver(protoregistry.GlobalFiles, protoregistry.GlobalTypes)
	refClient.AllowMissingFileDescriptors()

	// Parse the full method name (e.g., "/package.Service/Method")
	// This is a simplified parser - production code may need more robust parsing
	var serviceName, methodName string
	if len(fullMethodName) > 0 && fullMethodName[0] == '/' {
		fullMethodName = fullMethodName[1:]
	}

	// Split on last '/'
	lastSlash := -1
	for i := len(fullMethodName) - 1; i >= 0; i-- {
		if fullMethodName[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == -1 {
		return nil, fmt.Errorf("invalid method name format: %s", fullMethodName)
	}

	serviceName = fullMethodName[:lastSlash]
	methodName = fullMethodName[lastSlash+1:]

	serviceDesc, err := refClient.ResolveService(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve service %s: %w", serviceName, err)
	}

	methodDesc := serviceDesc.FindMethodByName(methodName)
	if methodDesc == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	// Convert jhump descriptor to protoreflect descriptor
	return methodDesc.UnwrapMethod(), nil
}

// GetMessageDescriptor returns the message descriptor for building requests.
// The messageName should be a fully qualified message type name.
func (c *Client) GetMessageDescriptor(ctx context.Context, messageName string) (protoreflect.MessageDescriptor, error) {
	c.logger.Debug("getting message descriptor", "message", messageName)

	refClient := grpcreflect.NewClientAuto(ctx, c.conn)
	defer refClient.Reset()

	refClient.AllowFallbackResolver(protoregistry.GlobalFiles, protoregistry.GlobalTypes)
	refClient.AllowMissingFileDescriptors()

	msgDesc, err := refClient.ResolveMessage(messageName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve message %s: %w", messageName, err)
	}

	// Convert jhump descriptor to protoreflect descriptor
	return msgDesc.UnwrapMessage(), nil
}

// convertServiceDescriptor converts a jhump ServiceDescriptor to domain.Service.
func (c *Client) convertServiceDescriptor(sd *desc.ServiceDescriptor) domain.Service {
	service := domain.Service{
		Name:     sd.GetName(),
		FullName: sd.GetFullyQualifiedName(),
	}

	methods := sd.GetMethods()
	for _, md := range methods {
		method := domain.Method{
			Name:           md.GetName(),
			FullName:       md.GetFullyQualifiedName(),
			InputType:      md.GetInputType().GetFullyQualifiedName(),
			OutputType:     md.GetOutputType().GetFullyQualifiedName(),
			IsClientStream: md.IsClientStreaming(),
			IsServerStream: md.IsServerStreaming(),
		}
		service.Methods = append(service.Methods, method)
	}

	return service
}
