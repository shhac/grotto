// Package main implements a gRPC test server that serves malformed
// FileDescriptorProtos via a custom reflection handler. It reproduces
// issues seen with servers that use non-canonical protobuf naming.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
)

func strPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32 { return &i }
func boolPtr(b bool) *bool    { return &b }

var (
	typeInt32     = descriptorpb.FieldDescriptorProto_TYPE_INT32
	typeInt64     = descriptorpb.FieldDescriptorProto_TYPE_INT64
	typeString    = descriptorpb.FieldDescriptorProto_TYPE_STRING
	typeMessage   = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	labelOptional = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRepeated = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
)

// buildGoogleProtobufFDP creates a non-canonical WKT barrel file.
// Instead of the standard google/protobuf/timestamp.proto etc., this server
// provides a single file "google_protobuf.proto" that defines both Timestamp
// and Duration in the google.protobuf package.
func buildGoogleProtobufFDP() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    strPtr("google_protobuf.proto"),
		Package: strPtr("google.protobuf"),
		Syntax:  strPtr("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Timestamp"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("seconds"), Number: int32Ptr(1), Type: &typeInt64, Label: &labelOptional},
					{Name: strPtr("nanos"), Number: int32Ptr(2), Type: &typeInt32, Label: &labelOptional},
				},
			},
			{
				Name: strPtr("Duration"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("seconds"), Number: int32Ptr(1), Type: &typeInt64, Label: &labelOptional},
					{Name: strPtr("nanos"), Number: int32Ptr(2), Type: &typeInt32, Label: &labelOptional},
				},
			},
		},
	}
}

// buildCustomTypesFDP creates a file with standalone message types.
func buildCustomTypesFDP() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    strPtr("custom_types.proto"),
		Package: strPtr("custom.types"),
		Syntax:  strPtr("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Money"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("amount"), Number: int32Ptr(1), Type: &typeInt64, Label: &labelOptional},
					{Name: strPtr("currency"), Number: int32Ptr(2), Type: &typeString, Label: &labelOptional},
				},
			},
			{
				Name: strPtr("Image"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("url"), Number: int32Ptr(1), Type: &typeString, Label: &labelOptional},
					{Name: strPtr("width"), Number: int32Ptr(2), Type: &typeInt32, Label: &labelOptional},
					{Name: strPtr("height"), Number: int32Ptr(3), Type: &typeInt32, Label: &labelOptional},
				},
			},
		},
	}
}

// buildCommonFDP creates a file with a non-canonical map entry name.
// The map field "key_values" generates entry "KeyValues" (no "Entry" suffix),
// reproducing a real-world server pattern.
func buildCommonFDP() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:       strPtr("common.proto"),
		Package:    strPtr("custom.common"),
		Syntax:     strPtr("proto3"),
		Dependency: []string{},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("DateValue"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("value"),
						Number:   int32Ptr(1),
						Type:     &typeMessage,
						TypeName: strPtr("google.protobuf.Timestamp"), // WKT - no leading dot
						Label:    &labelOptional,
					},
				},
			},
			{
				Name: strPtr("KeyValues"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("key_values"),
						Number:   int32Ptr(1),
						Type:     &typeMessage,
						TypeName: strPtr("KeyValues"), // relative, no "Entry" suffix
						Label:    &labelRepeated,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("KeyValues"), // no "Entry" suffix
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &typeString, Label: &labelOptional},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &typeString, Label: &labelOptional},
						},
					},
				},
			},
		},
	}
}

// buildEventServiceFDP creates the service file with EMPTY dependency array
// despite referencing types from google_protobuf.proto, custom_types.proto,
// and common.proto. It also has a non-canonical map entry name for events_by_org.
func buildEventServiceFDP() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:       strPtr("event_service.proto"),
		Package:    strPtr("custom.event.v1"),
		Syntax:     strPtr("proto3"),
		Dependency: []string{}, // EMPTY - missing all imports!
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Event"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("name"), Number: int32Ptr(1), Type: &typeString, Label: &labelOptional},
					{
						Name:     strPtr("created_at"),
						Number:   int32Ptr(2),
						Type:     &typeMessage,
						TypeName: strPtr("google.protobuf.Timestamp"), // WKT - no leading dot
						Label:    &labelOptional,
					},
					{
						Name:     strPtr("price"),
						Number:   int32Ptr(3),
						Type:     &typeMessage,
						TypeName: strPtr("types.Money"), // cross-file - short prefix
						Label:    &labelOptional,
					},
					{
						Name:     strPtr("date"),
						Number:   int32Ptr(4),
						Type:     &typeMessage,
						TypeName: strPtr("common.DateValue"), // cross-file - short prefix
						Label:    &labelOptional,
					},
				},
			},
			{
				Name: strPtr("GetEventRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &typeString, Label: &labelOptional},
				},
			},
			{
				Name: strPtr("GetEventsResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("events"),
						Number:   int32Ptr(1),
						Type:     &typeMessage,
						TypeName: strPtr("Event"), // same-file - bare name
						Label:    &labelRepeated,
					},
				},
			},
			{
				Name: strPtr("EventsByOrg"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("events_by_org"),
						Number:   int32Ptr(1),
						Type:     &typeMessage,
						TypeName: strPtr("EventsByOrg"), // relative, no "Entry" suffix
						Label:    &labelRepeated,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("EventsByOrg"), // no "Entry" suffix
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &typeString, Label: &labelOptional},
							{
								Name:     strPtr("value"),
								Number:   int32Ptr(2),
								Type:     &typeMessage,
								TypeName: strPtr("Event"), // same-file - bare name
								Label:    &labelOptional,
							},
						},
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: strPtr("EventService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       strPtr("GetEvent"),
						InputType:  strPtr(".custom.event.v1.GetEventRequest"),
						OutputType: strPtr(".custom.event.v1.Event"),
					},
					{
						Name:       strPtr("GetEvents"),
						InputType:  strPtr(".custom.event.v1.GetEventRequest"),
						OutputType: strPtr(".custom.event.v1.GetEventsResponse"),
					},
				},
			},
		},
	}
}

func marshalFDP(fdp *descriptorpb.FileDescriptorProto) []byte {
	data, err := proto.Marshal(fdp)
	if err != nil {
		log.Fatalf("failed to marshal FDP %s: %v", fdp.GetName(), err)
	}
	return data
}

func getHealthFDP() []byte {
	fd, err := protoregistry.GlobalFiles.FindFileByPath("grpc/health/v1/health.proto")
	if err != nil {
		log.Fatalf("failed to find health proto: %v", err)
	}
	fdp := protodesc.ToFileDescriptorProto(fd)
	data, err := proto.Marshal(fdp)
	if err != nil {
		log.Fatalf("failed to marshal health FDP: %v", err)
	}
	return data
}

// noncanonicalReflectionServer implements grpc_reflection_v1.ServerReflectionServer
// with hand-crafted malformed FDPs instead of using the standard reflection.Register.
type noncanonicalReflectionServer struct {
	reflectionpb.UnimplementedServerReflectionServer
	fdpsByName   map[string][]byte
	allEventFDPs [][]byte
	healthFDP    []byte
}

func newReflectionServer() *noncanonicalReflectionServer {
	googleProtobufBytes := marshalFDP(buildGoogleProtobufFDP())
	customTypesBytes := marshalFDP(buildCustomTypesFDP())
	commonBytes := marshalFDP(buildCommonFDP())
	eventServiceBytes := marshalFDP(buildEventServiceFDP())
	healthBytes := getHealthFDP()

	return &noncanonicalReflectionServer{
		fdpsByName: map[string][]byte{
			"google_protobuf.proto": googleProtobufBytes,
			"custom_types.proto":    customTypesBytes,
			"common.proto":          commonBytes,
			"event_service.proto":   eventServiceBytes,
		},
		allEventFDPs: [][]byte{
			eventServiceBytes,
			googleProtobufBytes,
			customTypesBytes,
			commonBytes,
		},
		healthFDP: healthBytes,
	}
}

func (s *noncanonicalReflectionServer) ServerReflectionInfo(stream grpc.BidiStreamingServer[reflectionpb.ServerReflectionRequest, reflectionpb.ServerReflectionResponse]) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		resp := &reflectionpb.ServerReflectionResponse{
			OriginalRequest: req,
		}

		switch req.MessageRequest.(type) {
		case *reflectionpb.ServerReflectionRequest_ListServices:
			resp.MessageResponse = &reflectionpb.ServerReflectionResponse_ListServicesResponse{
				ListServicesResponse: &reflectionpb.ListServiceResponse{
					Service: []*reflectionpb.ServiceResponse{
						{Name: "custom.event.v1.EventService"},
						{Name: "grpc.health.v1.Health"},
					},
				},
			}

		case *reflectionpb.ServerReflectionRequest_FileContainingSymbol:
			symbol := req.GetFileContainingSymbol()
			if strings.HasPrefix(symbol, "custom.") || strings.HasPrefix(symbol, "google.protobuf.") {
				// Return all 4 FDPs for any custom or google.protobuf symbol
				resp.MessageResponse = &reflectionpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &reflectionpb.FileDescriptorResponse{
						FileDescriptorProto: s.allEventFDPs,
					},
				}
			} else if strings.HasPrefix(symbol, "grpc.health.v1.") {
				resp.MessageResponse = &reflectionpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &reflectionpb.FileDescriptorResponse{
						FileDescriptorProto: [][]byte{s.healthFDP},
					},
				}
			} else {
				resp.MessageResponse = &reflectionpb.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &reflectionpb.ErrorResponse{
						ErrorCode:    int32(codes.NotFound),
						ErrorMessage: fmt.Sprintf("symbol not found: %s", symbol),
					},
				}
			}

		case *reflectionpb.ServerReflectionRequest_FileByFilename:
			filename := req.GetFileByFilename()
			if data, ok := s.fdpsByName[filename]; ok {
				resp.MessageResponse = &reflectionpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &reflectionpb.FileDescriptorResponse{
						FileDescriptorProto: [][]byte{data},
					},
				}
			} else if filename == "grpc/health/v1/health.proto" {
				resp.MessageResponse = &reflectionpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &reflectionpb.FileDescriptorResponse{
						FileDescriptorProto: [][]byte{s.healthFDP},
					},
				}
			} else {
				resp.MessageResponse = &reflectionpb.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &reflectionpb.ErrorResponse{
						ErrorCode:    int32(codes.NotFound),
						ErrorMessage: fmt.Sprintf("file not found: %s", filename),
					},
				}
			}

		default:
			resp.MessageResponse = &reflectionpb.ServerReflectionResponse_ErrorResponse{
				ErrorResponse: &reflectionpb.ErrorResponse{
					ErrorCode:    int32(codes.Unimplemented),
					ErrorMessage: "request type not supported",
				},
			}
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func main() {
	addr := flag.String("addr", "localhost:50055", "listen address")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()

	// Register custom reflection server (NOT standard reflection.Register)
	reflectionpb.RegisterServerReflectionServer(s, newReflectionServer())

	// Register standard health service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	log.Printf("Non-canonical gRPC test server listening on %s", *addr)
	log.Printf("Services: custom.event.v1.EventService, grpc.health.v1.Health")
	log.Printf("Custom reflection handler (serves malformed FDPs)")

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
