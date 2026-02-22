package grpc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jhump/protoreflect/v2/grpcreflect"
	"github.com/shhac/grotto/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// ReflectionClient wraps gRPC server reflection functionality
type ReflectionClient struct {
	conn         *grpc.ClientConn
	client       *grpcreflect.Client
	logger       *slog.Logger
	serviceCache map[string]protoreflect.ServiceDescriptor
}

// NewReflectionClient creates a new reflection client for the given connection
func NewReflectionClient(conn *grpc.ClientConn, logger *slog.Logger) *ReflectionClient {
	// Use NewClientAuto which takes the connection directly
	refClient := grpcreflect.NewClientAuto(context.Background(), conn,
		grpcreflect.WithAllowMissingFileDescriptors(),
		grpcreflect.WithFallbackResolvers(protoregistry.GlobalFiles, protoregistry.GlobalTypes),
	)

	return &ReflectionClient{
		conn:         conn,
		client:       refClient,
		logger:       logger,
		serviceCache: make(map[string]protoreflect.ServiceDescriptor),
	}
}

// ListServices discovers all services available on the server
func (r *ReflectionClient) ListServices(ctx context.Context) ([]domain.Service, error) {
	r.logger.Debug("listing services via reflection")

	serviceNames, err := r.client.ListServices()
	if err != nil {
		r.logger.Error("failed to list services", slog.Any("error", err))
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	resolver := r.client.AsResolver()

	var services []domain.Service
	for _, serviceName := range serviceNames {
		// Skip reflection service itself
		if serviceName == "grpc.reflection.v1alpha.ServerReflection" ||
			serviceName == "grpc.reflection.v1.ServerReflection" {
			continue
		}

		// Load the file containing this service (populates the resolver cache)
		_, err := r.client.FileContainingSymbol(serviceName)
		if err != nil {
			r.logger.Warn("failed to load service file, skipping",
				slog.String("service", string(serviceName)),
				slog.Any("error", err),
			)
			continue
		}

		// Resolve the service descriptor
		desc, err := resolver.FindDescriptorByName(serviceName)
		if err != nil {
			r.logger.Warn("failed to resolve service, skipping",
				slog.String("service", string(serviceName)),
				slog.Any("error", err),
			)
			continue
		}

		serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
		if !ok {
			r.logger.Warn("descriptor is not a service, skipping",
				slog.String("service", string(serviceName)),
			)
			continue
		}

		r.serviceCache[string(serviceName)] = serviceDesc
		service := r.convertService(serviceDesc)
		services = append(services, service)
	}

	r.logger.Info("discovered services via reflection",
		slog.Int("service_count", len(services)),
	)

	return services, nil
}

// GetMethodDescriptor returns the descriptor for a specific method
func (r *ReflectionClient) GetMethodDescriptor(serviceName, methodName string) (protoreflect.MethodDescriptor, error) {
	serviceDesc, ok := r.serviceCache[serviceName]
	if !ok {
		// Load the file and resolve the service descriptor
		resolver := r.client.AsResolver()
		_, err := r.client.FileContainingSymbol(protoreflect.FullName(serviceName))
		if err != nil {
			return nil, fmt.Errorf("failed to load service %s: %w", serviceName, err)
		}
		d, err := resolver.FindDescriptorByName(protoreflect.FullName(serviceName))
		if err != nil {
			return nil, fmt.Errorf("failed to resolve service %s: %w", serviceName, err)
		}
		sd, ok := d.(protoreflect.ServiceDescriptor)
		if !ok {
			return nil, fmt.Errorf("descriptor for %s is not a service", serviceName)
		}
		serviceDesc = sd
		r.serviceCache[serviceName] = serviceDesc
	}

	methodDesc := serviceDesc.Methods().ByName(protoreflect.Name(methodName))
	if methodDesc == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	return methodDesc, nil
}

// Close closes the reflection client
func (r *ReflectionClient) Close() {
	r.client.Reset()
	r.serviceCache = nil
}

// convertService converts a protoreflect ServiceDescriptor to domain.Service
func (r *ReflectionClient) convertService(sd protoreflect.ServiceDescriptor) domain.Service {
	methods := sd.Methods()
	service := domain.Service{
		Name:     string(sd.Name()),
		FullName: string(sd.FullName()),
		Methods:  make([]domain.Method, 0, methods.Len()),
	}

	for i := range methods.Len() {
		md := methods.Get(i)
		method := domain.Method{
			Name:           string(md.Name()),
			FullName:       string(md.FullName()),
			InputType:      string(md.Input().FullName()),
			OutputType:     string(md.Output().FullName()),
			IsClientStream: md.IsStreamingClient(),
			IsServerStream: md.IsStreamingServer(),
		}
		service.Methods = append(service.Methods, method)
	}

	return service
}
