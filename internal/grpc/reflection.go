package grpc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/shhac/grotto/internal/domain"
	"google.golang.org/grpc"
)

// ReflectionClient wraps gRPC server reflection functionality
type ReflectionClient struct {
	conn         *grpc.ClientConn
	client       *grpcreflect.Client
	logger       *slog.Logger
	serviceCache map[string]*desc.ServiceDescriptor
}

// NewReflectionClient creates a new reflection client for the given connection
func NewReflectionClient(conn *grpc.ClientConn, logger *slog.Logger) *ReflectionClient {
	// Use NewClientAuto which takes the connection directly
	refClient := grpcreflect.NewClientAuto(context.Background(), conn)

	return &ReflectionClient{
		conn:         conn,
		client:       refClient,
		logger:       logger,
		serviceCache: make(map[string]*desc.ServiceDescriptor),
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

	var services []domain.Service
	for _, serviceName := range serviceNames {
		// Skip reflection service itself
		if serviceName == "grpc.reflection.v1alpha.ServerReflection" ||
			serviceName == "grpc.reflection.v1.ServerReflection" {
			continue
		}

		serviceDesc, err := r.client.ResolveService(serviceName)
		if err != nil {
			r.logger.Warn("failed to resolve service, skipping",
				slog.String("service", serviceName),
				slog.Any("error", err),
			)
			continue
		}

		r.serviceCache[serviceName] = serviceDesc
		service := r.convertService(serviceDesc)
		services = append(services, service)
	}

	r.logger.Info("discovered services via reflection",
		slog.Int("service_count", len(services)),
	)

	return services, nil
}

// GetMethodDescriptor returns the descriptor for a specific method
func (r *ReflectionClient) GetMethodDescriptor(serviceName, methodName string) (*desc.MethodDescriptor, error) {
	serviceDesc, ok := r.serviceCache[serviceName]
	if !ok {
		var err error
		serviceDesc, err = r.client.ResolveService(serviceName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve service %s: %w", serviceName, err)
		}
		r.serviceCache[serviceName] = serviceDesc
	}

	methodDesc := serviceDesc.FindMethodByName(methodName)
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
func (r *ReflectionClient) convertService(sd *desc.ServiceDescriptor) domain.Service {
	service := domain.Service{
		Name:     sd.GetName(),
		FullName: sd.GetFullyQualifiedName(),
		Methods:  make([]domain.Method, 0, len(sd.GetMethods())),
	}

	for _, md := range sd.GetMethods() {
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
