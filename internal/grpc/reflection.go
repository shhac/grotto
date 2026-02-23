package grpc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jhump/protoreflect/v2/grpcreflect"
	"github.com/shhac/grotto/internal/domain"
	"google.golang.org/grpc"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
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
			r.logger.Warn("standard resolution failed, trying lenient resolve",
				slog.String("service", string(serviceName)),
				slog.Any("error", err),
			)

			// Try lenient resolution with AllowUnresolvable
			sd, lenientErr := r.lenientResolve(ctx, string(serviceName))
			if lenientErr != nil {
				r.logger.Warn("lenient resolution also failed",
					slog.String("service", string(serviceName)),
					slog.Any("error", lenientErr),
				)
				services = append(services, domain.Service{
					Name:     string(serviceName.Name()),
					FullName: string(serviceName),
					Error:    err.Error(),
				})
				continue
			}

			r.serviceCache[string(serviceName)] = sd
			service := r.convertService(sd)
			services = append(services, service)
			r.logger.Info("lenient resolution succeeded",
				slog.String("service", string(serviceName)),
				slog.Int("methods", len(service.Methods)),
			)
			continue
		}

		// Resolve the service descriptor
		desc, err := resolver.FindDescriptorByName(serviceName)
		if err != nil {
			r.logger.Warn("failed to resolve service",
				slog.String("service", string(serviceName)),
				slog.Any("error", err),
			)
			services = append(services, domain.Service{
				Name:     string(serviceName.Name()),
				FullName: string(serviceName),
				Error:    err.Error(),
			})
			continue
		}

		serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
		if !ok {
			r.logger.Warn("descriptor is not a service",
				slog.String("service", string(serviceName)),
			)
			services = append(services, domain.Service{
				Name:     string(serviceName.Name()),
				FullName: string(serviceName),
				Error:    "descriptor is not a service",
			})
			continue
		}

		r.serviceCache[string(serviceName)] = serviceDesc
		service := r.convertService(serviceDesc)
		services = append(services, service)
	}

	// Log summary with error count
	errorCount := 0
	for _, s := range services {
		if s.Error != "" {
			errorCount++
		}
	}
	if errorCount > 0 {
		r.logger.Warn("some services failed descriptor resolution",
			slog.Int("total", len(services)),
			slog.Int("errors", errorCount),
		)
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

// lenientResolve uses the raw reflection protocol with protodesc.AllowUnresolvable
// to build service descriptors even when some type dependencies can't be resolved.
func (r *ReflectionClient) lenientResolve(ctx context.Context, serviceName string) (protoreflect.ServiceDescriptor, error) {
	refClient := reflectionpb.NewServerReflectionClient(r.conn)
	stream, err := refClient.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open reflection stream: %w", err)
	}
	defer stream.CloseSend()

	// Request file containing the service symbol
	if err := stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: serviceName,
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to send reflection request: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive reflection response: %w", err)
	}

	fdResp := resp.GetFileDescriptorResponse()
	if fdResp == nil {
		if errResp := resp.GetErrorResponse(); errResp != nil {
			return nil, fmt.Errorf("reflection error: %s", errResp.GetErrorMessage())
		}
		return nil, fmt.Errorf("unexpected reflection response type")
	}

	// Parse all returned file descriptor protos
	var fdProtos []*descriptorpb.FileDescriptorProto
	seen := map[string]bool{}
	for _, raw := range fdResp.GetFileDescriptorProto() {
		fd := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(raw, fd); err != nil {
			r.logger.Warn("failed to unmarshal file descriptor in lenient resolve", slog.Any("error", err))
			continue
		}
		if !seen[fd.GetName()] {
			fdProtos = append(fdProtos, fd)
			seen[fd.GetName()] = true
		}
	}

	// Fetch missing dependencies not available locally
	needed := map[string]bool{}
	for _, fd := range fdProtos {
		for _, dep := range fd.GetDependency() {
			if !seen[dep] {
				if _, err := protoregistry.GlobalFiles.FindFileByPath(dep); err != nil {
					needed[dep] = true
				}
			}
		}
	}

	for dep := range needed {
		if err := stream.Send(&reflectionpb.ServerReflectionRequest{
			MessageRequest: &reflectionpb.ServerReflectionRequest_FileByFilename{
				FileByFilename: dep,
			},
		}); err != nil {
			r.logger.Debug("failed to request dependency file",
				slog.String("dep", dep), slog.Any("error", err))
			continue
		}
		depResp, err := stream.Recv()
		if err != nil {
			r.logger.Debug("failed to receive dependency file",
				slog.String("dep", dep), slog.Any("error", err))
			continue
		}
		if depFdResp := depResp.GetFileDescriptorResponse(); depFdResp != nil {
			for _, raw := range depFdResp.GetFileDescriptorProto() {
				fd := &descriptorpb.FileDescriptorProto{}
				if err := proto.Unmarshal(raw, fd); err == nil && !seen[fd.GetName()] {
					fdProtos = append(fdProtos, fd)
					seen[fd.GetName()] = true
				}
			}
		}
	}

	// Build file descriptors with lenient options
	opts := protodesc.FileOptions{AllowUnresolvable: true}
	localFiles := new(protoregistry.Files)
	resolver := &combinedResolver{local: localFiles, global: protoregistry.GlobalFiles}

	// Iteratively parse files (deps before dependents)
	remaining := make([]*descriptorpb.FileDescriptorProto, len(fdProtos))
	copy(remaining, fdProtos)

	var serviceDesc protoreflect.ServiceDescriptor
	for len(remaining) > 0 {
		progress := false
		var next []*descriptorpb.FileDescriptorProto

		for _, fd := range remaining {
			// Skip files already registered
			if _, err := localFiles.FindFileByPath(fd.GetName()); err == nil {
				progress = true
				continue
			}
			if _, err := protoregistry.GlobalFiles.FindFileByPath(fd.GetName()); err == nil {
				progress = true
				continue
			}

			parsed, err := opts.New(fd, resolver)
			if err != nil {
				next = append(next, fd)
				continue
			}
			progress = true
			if regErr := localFiles.RegisterFile(parsed); regErr != nil {
				r.logger.Debug("failed to register lenient file",
					slog.String("file", fd.GetName()),
					slog.Any("error", regErr),
				)
				continue
			}

			// Check if this file contains our service
			for i := range parsed.Services().Len() {
				sd := parsed.Services().Get(i)
				if string(sd.FullName()) == serviceName {
					serviceDesc = sd
				}
			}
		}

		remaining = next
		if !progress {
			break
		}
	}

	if serviceDesc == nil {
		return nil, fmt.Errorf("service %s not found after lenient parsing", serviceName)
	}

	return serviceDesc, nil
}

// combinedResolver tries local files first, then falls back to global registry.
type combinedResolver struct {
	local  *protoregistry.Files
	global *protoregistry.Files
}

func (r *combinedResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	if fd, err := r.local.FindFileByPath(path); err == nil {
		return fd, nil
	}
	return r.global.FindFileByPath(path)
}

func (r *combinedResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	if d, err := r.local.FindDescriptorByName(name); err == nil {
		return d, nil
	}
	return r.global.FindDescriptorByName(name)
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
