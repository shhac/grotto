package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

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
					Error:    fmt.Sprintf("%s\n\nLenient: %s", err.Error(), lenientErr.Error()),
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

	for _, fd := range fdProtos {
		r.logger.Debug("lenient resolve: received file descriptor",
			slog.String("file", fd.GetName()),
			slog.String("package", fd.GetPackage()),
			slog.Any("deps", fd.GetDependency()),
		)
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

	localFiles, err := buildFileDescriptors(fdProtos, r.logger)
	if err != nil {
		return nil, err
	}

	// Search the built registry for the target service
	var serviceDesc protoreflect.ServiceDescriptor
	localFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := range fd.Services().Len() {
			sd := fd.Services().Get(i)
			if string(sd.FullName()) == serviceName {
				serviceDesc = sd
				return false
			}
		}
		return true
	})

	if serviceDesc == nil {
		return nil, fmt.Errorf("service %s not found after lenient parsing", serviceName)
	}

	return serviceDesc, nil
}

// buildFileDescriptors iteratively builds protoreflect FileDescriptors from raw
// FileDescriptorProtos using lenient options. It handles dependency ordering and
// fixes missing imports on failure. Returns the registry of successfully built files.
func buildFileDescriptors(fdProtos []*descriptorpb.FileDescriptorProto, logger *slog.Logger) (*protoregistry.Files, error) {
	opts := protodesc.FileOptions{AllowUnresolvable: true}
	localFiles := new(protoregistry.Files)
	resolver := &combinedResolver{local: localFiles, global: protoregistry.GlobalFiles}

	// Pre-fix malformed descriptors before building
	for _, fd := range fdProtos {
		if fixMapEntryNames(fd) {
			logger.Debug("fixed malformed map entry names",
				slog.String("file", fd.GetName()),
			)
		}
		if fixReservedRanges(fd) {
			logger.Debug("fixed malformed reserved ranges",
				slog.String("file", fd.GetName()),
			)
		}
	}

	remaining := make([]*descriptorpb.FileDescriptorProto, len(fdProtos))
	copy(remaining, fdProtos)

	iteration := 0
	for len(remaining) > 0 {
		iteration++
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
				firstErr := err
				if fixMissingImports(fd, resolver, logger) {
					logger.Debug("fixMissingImports: injected imports",
						slog.String("file", fd.GetName()),
						slog.Any("deps", fd.GetDependency()),
					)
					parsed, err = opts.New(fd, resolver)
					if err != nil {
						logger.Debug("build still failed after import fix",
							slog.String("file", fd.GetName()),
							slog.String("first_error", firstErr.Error()),
							slog.String("retry_error", err.Error()),
						)
					}
				}
			}
			if err != nil {
				next = append(next, fd)
				continue
			}
			progress = true
			if regErr := localFiles.RegisterFile(parsed); regErr != nil {
				logger.Debug("failed to register lenient file",
					slog.String("file", fd.GetName()),
					slog.Any("error", regErr),
				)
				continue
			}
			logger.Debug("successfully built file",
				slog.String("file", fd.GetName()),
				slog.Int("iteration", iteration),
			)
		}

		remaining = next
		if !progress {
			for _, fd := range remaining {
				_, lastErr := opts.New(fd, resolver)
				logger.Warn("file stuck after all retries",
					slog.String("file", fd.GetName()),
					slog.Any("deps", fd.GetDependency()),
					slog.Any("error", lastErr),
					slog.Int("local_files", localFiles.NumFiles()),
				)
			}
			break
		}
	}

	if localFiles.NumFiles() == 0 {
		return nil, fmt.Errorf("no files could be built from %d protos", len(fdProtos))
	}

	return localFiles, nil
}

// combinedResolver merges local (server-provided) files with the global registry.
// FindFileByPath checks local first (server files may have non-canonical paths).
// FindDescriptorByName checks global first so canonical type definitions always
// win over non-canonical server duplicates (e.g., google_protobuf.proto defining
// google.protobuf.Timestamp should not shadow google/protobuf/timestamp.proto).
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
	if d, err := r.global.FindDescriptorByName(name); err == nil {
		return d, nil
	}
	return r.local.FindDescriptorByName(name)
}

// fixMissingImports scans a FileDescriptorProto for type references, resolves
// them against the given resolver, and adds any missing parent file imports.
// This handles servers that reference types without declaring the corresponding
// import, which causes protodesc.NewFile to fail with "resolved X, but Y is
// not imported". Returns true if any imports were added.
//
// Type references may be fully-qualified (".pkg.Type") or relative ("Type",
// "sub.Type"). Relative refs are resolved using proto scoping rules: the file's
// package is progressively stripped to find a matching fully-qualified name.
func fixMissingImports(fd *descriptorpb.FileDescriptorProto, r protodesc.Resolver, logger *slog.Logger) bool {
	existing := make(map[string]bool, len(fd.GetDependency()))
	for _, d := range fd.GetDependency() {
		existing[d] = true
	}

	refs := collectTypeRefs(fd)
	logger.Debug("fixMissingImports: collected type refs",
		slog.String("file", fd.GetName()),
		slog.Int("ref_count", len(refs)),
		slog.Any("refs", refs),
	)

	pkg := fd.GetPackage()
	added := false
	for _, ref := range refs {
		name := strings.TrimPrefix(ref, ".")
		if name == "" {
			continue
		}
		d := resolveTypeRef(name, pkg, r)
		if d == nil {
			continue
		}
		filePath := d.ParentFile().Path()
		if !existing[filePath] {
			logger.Debug("fixMissingImports: adding import",
				slog.String("file", fd.GetName()),
				slog.String("type", name),
				slog.String("resolved_from", string(filePath)),
			)
			fd.Dependency = append(fd.Dependency, filePath)
			existing[filePath] = true
			added = true
		}
	}
	return added
}

// resolveTypeRef resolves a type reference that may be relative using proto
// scoping rules. For a relative ref like "types.Money" in package "acme.svc.v1",
// it tries: acme.svc.v1.types.Money, acme.svc.types.Money, acme.types.Money,
// types.Money — returning the first match.
func resolveTypeRef(name, pkg string, r protodesc.Resolver) protoreflect.Descriptor {
	// Try as-is first (handles already-qualified names like "google.protobuf.Timestamp")
	if d, err := r.FindDescriptorByName(protoreflect.FullName(name)); err == nil {
		return d
	}
	// Try with package prefix scoping: prepend progressively shorter package prefixes
	for pkg != "" {
		candidate := pkg + "." + name
		if d, err := r.FindDescriptorByName(protoreflect.FullName(candidate)); err == nil {
			return d
		}
		// Strip the last package segment
		if i := strings.LastIndex(pkg, "."); i >= 0 {
			pkg = pkg[:i]
		} else {
			break
		}
	}
	return nil
}

// collectTypeRefs collects all type name references from a FileDescriptorProto,
// including nested messages (which covers map entry types), extensions, and
// service method input/output types.
func collectTypeRefs(fd *descriptorpb.FileDescriptorProto) []string {
	var refs []string
	for _, msg := range fd.GetMessageType() {
		collectMessageTypeRefs(msg, &refs)
	}
	for _, ext := range fd.GetExtension() {
		collectFieldTypeRef(ext, &refs)
	}
	for _, svc := range fd.GetService() {
		for _, m := range svc.GetMethod() {
			if t := m.GetInputType(); t != "" {
				refs = append(refs, t)
			}
			if t := m.GetOutputType(); t != "" {
				refs = append(refs, t)
			}
		}
	}
	return refs
}

// collectMessageTypeRefs recursively collects type references from a message,
// including its fields, extensions, and nested types.
func collectMessageTypeRefs(msg *descriptorpb.DescriptorProto, refs *[]string) {
	for _, field := range msg.GetField() {
		collectFieldTypeRef(field, refs)
	}
	for _, ext := range msg.GetExtension() {
		collectFieldTypeRef(ext, refs)
	}
	for _, nested := range msg.GetNestedType() {
		collectMessageTypeRefs(nested, refs)
	}
}

// collectFieldTypeRef appends a field's TypeName and Extendee (if non-empty) to refs.
func collectFieldTypeRef(field *descriptorpb.FieldDescriptorProto, refs *[]string) {
	if t := field.GetTypeName(); t != "" {
		*refs = append(*refs, t)
	}
	if t := field.GetExtendee(); t != "" {
		*refs = append(*refs, t)
	}
}

// fixMapEntryNames fixes malformed map entry message names in a FileDescriptorProto.
// Some servers (e.g., those using non-standard proto tooling) produce map entries
// with names that don't match protobuf's expected convention of CamelCase(field_name)+"Entry".
// For example, a field "competitions" might have entry "CompetitionEntry" instead of
// "CompetitionsEntry". protodesc rejects these with "incorrect implicit map entry name".
func fixMapEntryNames(fd *descriptorpb.FileDescriptorProto) bool {
	pkg := fd.GetPackage()
	fixed := false
	for _, msg := range fd.GetMessageType() {
		fqn := pkg
		if fqn != "" {
			fqn += "."
		}
		fqn += msg.GetName()
		if fixMapEntriesInMessage(msg, fqn) {
			fixed = true
		}
	}
	return fixed
}

func fixMapEntriesInMessage(msg *descriptorpb.DescriptorProto, fqn string) bool {
	fixed := false

	// Recurse into nested types (non-map-entry messages can also have map fields)
	for _, nested := range msg.GetNestedType() {
		if nested.GetOptions().GetMapEntry() {
			continue
		}
		nestedFQN := fqn + "." + nested.GetName()
		if fixMapEntriesInMessage(nested, nestedFQN) {
			fixed = true
		}
	}

	// For each field, check if it references a map entry with wrong name.
	// TypeNames may be fully-qualified (".pkg.Msg.Entry") or relative ("Entry",
	// "Msg.Entry") depending on the server's proto tooling.
	for _, field := range msg.GetField() {
		typeName := field.GetTypeName()
		if typeName == "" {
			continue
		}

		// Find the map entry nested type this field references
		for _, nested := range msg.GetNestedType() {
			if !nested.GetOptions().GetMapEntry() {
				continue
			}
			entryName := nested.GetName()
			absRef := "." + fqn + "." + entryName

			// Match both absolute and relative TypeName forms
			if typeName != absRef && typeName != entryName &&
				!strings.HasSuffix(absRef, "."+typeName) {
				continue
			}

			// This field references this map entry. Check the name.
			expectedName := mapEntryName(field.GetName())
			if entryName == expectedName {
				break // already correct
			}
			// Fix the entry name
			nested.Name = &expectedName
			// Update field's TypeName, preserving relative/absolute form
			if typeName == absRef {
				correctRef := "." + fqn + "." + expectedName
				field.TypeName = &correctRef
			} else {
				// Replace the last path component (the entry name) with the corrected one
				correctRef := strings.TrimSuffix(typeName, entryName) + expectedName
				field.TypeName = &correctRef
			}
			fixed = true
			break
		}
	}
	return fixed
}

// fixReservedRanges fixes invalid reserved ranges in all messages of a FileDescriptorProto.
// Some servers produce ranges where end <= start (e.g., start=2, end=2), which is invalid
// because protobuf reserved ranges are end-exclusive: [start, end). We fix these by
// setting end = start + 1 to reserve the single field number.
func fixReservedRanges(fd *descriptorpb.FileDescriptorProto) bool {
	fixed := false
	for _, msg := range fd.GetMessageType() {
		if fixReservedRangesInMessage(msg) {
			fixed = true
		}
	}
	return fixed
}

func fixReservedRangesInMessage(msg *descriptorpb.DescriptorProto) bool {
	fixed := false
	for _, r := range msg.GetReservedRange() {
		if r.GetEnd() <= r.GetStart() {
			corrected := r.GetStart() + 1
			r.End = &corrected
			fixed = true
		}
	}
	for _, nested := range msg.GetNestedType() {
		if fixReservedRangesInMessage(nested) {
			fixed = true
		}
	}
	return fixed
}

// mapEntryName computes the expected map entry message name for a field,
// matching protobuf's convention: capitalize each underscore-separated segment
// and append "Entry". E.g., "foo_bar" → "FooBarEntry".
func mapEntryName(fieldName string) string {
	var b []byte
	upperNext := true
	for _, c := range fieldName {
		if c == '_' {
			upperNext = true
			continue
		}
		if upperNext {
			b = append(b, byte(unicode.ToUpper(c)))
			upperNext = false
		} else {
			b = append(b, byte(c))
		}
	}
	return string(b) + "Entry"
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
