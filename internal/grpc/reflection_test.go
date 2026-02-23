package grpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/shhac/grotto/internal/domain"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// --- fixMissingImports unit tests ---

func TestFixMissingImports_AddsResolvableImport(t *testing.T) {
	typeName := ".google.protobuf.Timestamp"
	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("MyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("created_at"),
						Number:   int32Ptr(1),
						TypeName: &typeName,
					},
				},
			},
		},
		Dependency: []string{},
	}

	added := fixMissingImports(fd, protoregistry.GlobalFiles, discardLogger)
	if !added {
		t.Fatal("expected fixMissingImports to return true")
	}

	depSet := make(map[string]bool)
	for _, d := range fd.Dependency {
		depSet[d] = true
	}

	if !depSet["google/protobuf/timestamp.proto"] {
		t.Error("expected google/protobuf/timestamp.proto in dependencies")
	}
}

func TestFixMissingImports_NoDuplicates(t *testing.T) {
	typeName := ".google.protobuf.Timestamp"
	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("MyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("created_at"),
						Number:   int32Ptr(1),
						TypeName: &typeName,
					},
				},
			},
		},
		Dependency: []string{
			"google/protobuf/timestamp.proto",
		},
	}

	added := fixMissingImports(fd, protoregistry.GlobalFiles, discardLogger)
	if added {
		t.Error("expected fixMissingImports to return false when import already exists")
	}

	counts := make(map[string]int)
	for _, d := range fd.Dependency {
		counts[d]++
	}
	if counts["google/protobuf/timestamp.proto"] != 1 {
		t.Errorf("expected google/protobuf/timestamp.proto exactly once, got %d", counts["google/protobuf/timestamp.proto"])
	}
}

func TestFixMissingImports_ReturnsFalseWhenNothingNeeded(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("MyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   strPtr("name"),
						Number: int32Ptr(1),
						// No TypeName — scalar field
					},
				},
			},
		},
		Dependency: []string{},
	}

	added := fixMissingImports(fd, protoregistry.GlobalFiles, discardLogger)
	if added {
		t.Error("expected fixMissingImports to return false for scalar-only fields")
	}
	if len(fd.Dependency) != 0 {
		t.Errorf("expected no dependencies added, got %v", fd.Dependency)
	}
}

func TestFixMissingImports_HandlesServiceMethods(t *testing.T) {
	inputType := ".google.protobuf.Timestamp"
	outputType := ".google.protobuf.Duration"
	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: strPtr("MyService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       strPtr("DoStuff"),
						InputType:  &inputType,
						OutputType: &outputType,
					},
				},
			},
		},
		Dependency: []string{},
	}

	added := fixMissingImports(fd, protoregistry.GlobalFiles, discardLogger)
	if !added {
		t.Fatal("expected fixMissingImports to return true")
	}

	depSet := make(map[string]bool)
	for _, d := range fd.Dependency {
		depSet[d] = true
	}

	if !depSet["google/protobuf/timestamp.proto"] {
		t.Error("expected google/protobuf/timestamp.proto in dependencies")
	}
	if !depSet["google/protobuf/duration.proto"] {
		t.Error("expected google/protobuf/duration.proto in dependencies")
	}
}

func TestFixMissingImports_HandlesNestedMessages(t *testing.T) {
	typeName := ".google.protobuf.Duration"
	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Outer"),
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: strPtr("Inner"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     strPtr("timeout"),
								Number:   int32Ptr(1),
								TypeName: &typeName,
							},
						},
					},
				},
			},
		},
		Dependency: []string{},
	}

	added := fixMissingImports(fd, protoregistry.GlobalFiles, discardLogger)
	if !added {
		t.Fatal("expected fixMissingImports to return true")
	}

	depSet := make(map[string]bool)
	for _, d := range fd.Dependency {
		depSet[d] = true
	}

	if !depSet["google/protobuf/duration.proto"] {
		t.Error("expected google/protobuf/duration.proto in dependencies")
	}
}

// --- buildFileDescriptors integration tests ---
// These test the full build loop including fixMissingImports retry logic.

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// makeServiceFDP creates a FileDescriptorProto mimicking a server's service file.
// The service has one method: GetItem(GetItemRequest) returns (Item).
// Item.created_at is of type .google.protobuf.Timestamp.
func makeServiceFDP(deps []string) *descriptorpb.FileDescriptorProto {
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	return &descriptorpb.FileDescriptorProto{
		Name:       strPtr("noncanonical_service.proto"),
		Syntax:     strPtr("proto3"),
		Package:    strPtr("test.noncanonical.v1"),
		Dependency: deps,
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Item"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &strType, Label: &label},
					{Name: strPtr("created_at"), Number: int32Ptr(2), Type: &msgType, Label: &label, TypeName: strPtr(".google.protobuf.Timestamp")},
				},
			},
			{
				Name: strPtr("GetItemRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &strType, Label: &label},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: strPtr("NonCanonicalService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       strPtr("GetItem"),
						InputType:  strPtr(".test.noncanonical.v1.GetItemRequest"),
						OutputType: strPtr(".test.noncanonical.v1.Item"),
					},
				},
			},
		},
	}
}

// makeNonCanonicalTimestampFDP creates a FileDescriptorProto at a non-canonical path
// that defines google.protobuf.Timestamp, mimicking servers that bundle WKTs
// with flat file names.
func makeNonCanonicalTimestampFDP() *descriptorpb.FileDescriptorProto {
	int64Type := descriptorpb.FieldDescriptorProto_TYPE_INT64
	int32Type := descriptorpb.FieldDescriptorProto_TYPE_INT32
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	return &descriptorpb.FileDescriptorProto{
		Name:    strPtr("google_protobuf.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("google.protobuf"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Timestamp"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("seconds"), Number: int32Ptr(1), Type: &int64Type, Label: &label},
					{Name: strPtr("nanos"), Number: int32Ptr(2), Type: &int32Type, Label: &label},
				},
			},
		},
	}
}

func findService(files *protoregistry.Files, fullName string) protoreflect.ServiceDescriptor {
	var result protoreflect.ServiceDescriptor
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := range fd.Services().Len() {
			sd := fd.Services().Get(i)
			if string(sd.FullName()) == fullName {
				result = sd
				return false
			}
		}
		return true
	})
	return result
}

func TestBuildFileDescriptors_CanonicalImport(t *testing.T) {
	// Baseline: service file correctly imports google/protobuf/timestamp.proto.
	// Should work without fixMissingImports since GlobalFiles has it.
	svcFDP := makeServiceFDP([]string{"google/protobuf/timestamp.proto"})

	files, err := buildFileDescriptors([]*descriptorpb.FileDescriptorProto{svcFDP}, discardLogger)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "test.noncanonical.v1.NonCanonicalService")
	if sd == nil {
		t.Fatal("expected to find EventService")
	}
	if sd.Methods().Len() != 1 {
		t.Errorf("expected 1 method, got %d", sd.Methods().Len())
	}
}

func TestBuildFileDescriptors_NonCanonicalWKTProvided(t *testing.T) {
	// Server provides google_protobuf.proto (non-canonical) that defines Timestamp.
	// Service file imports google_protobuf.proto.
	// Build loop should register the non-canonical file first, then resolve the service.
	wktFDP := makeNonCanonicalTimestampFDP()
	svcFDP := makeServiceFDP([]string{"google_protobuf.proto"})

	files, err := buildFileDescriptors([]*descriptorpb.FileDescriptorProto{svcFDP, wktFDP}, discardLogger)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "test.noncanonical.v1.NonCanonicalService")
	if sd == nil {
		t.Fatal("expected to find EventService")
	}
	if sd.Methods().Len() != 1 {
		t.Errorf("expected 1 method, got %d", sd.Methods().Len())
	}
}

func TestBuildFileDescriptors_MissingImportEntirely(t *testing.T) {
	// Service file uses Timestamp but has NO import at all.
	// fixMissingImports should add google/protobuf/timestamp.proto from GlobalFiles.
	svcFDP := makeServiceFDP(nil)

	files, err := buildFileDescriptors([]*descriptorpb.FileDescriptorProto{svcFDP}, discardLogger)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "test.noncanonical.v1.NonCanonicalService")
	if sd == nil {
		t.Fatal("expected to find EventService")
	}
}

func TestBuildFileDescriptors_NonCanonicalWKTNotProvided(t *testing.T) {
	// Service file imports google_protobuf.proto (non-canonical) but that file
	// is NOT provided. fixMissingImports should add canonical import from GlobalFiles.
	svcFDP := makeServiceFDP([]string{"google_protobuf.proto"})

	files, err := buildFileDescriptors([]*descriptorpb.FileDescriptorProto{svcFDP}, discardLogger)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "test.noncanonical.v1.NonCanonicalService")
	if sd == nil {
		t.Fatal("expected to find EventService")
	}
}

func TestBuildFileDescriptors_MultipleWKTs(t *testing.T) {
	// Service file references Timestamp AND Duration, with no imports.
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	svcFDP := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("multi.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("test.multi"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Event"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("name"), Number: int32Ptr(1), Type: &strType, Label: &label},
					{Name: strPtr("created_at"), Number: int32Ptr(2), Type: &msgType, Label: &label, TypeName: strPtr(".google.protobuf.Timestamp")},
					{Name: strPtr("duration"), Number: int32Ptr(3), Type: &msgType, Label: &label, TypeName: strPtr(".google.protobuf.Duration")},
				},
			},
			{
				Name: strPtr("GetEventRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &strType, Label: &label},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: strPtr("EventService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       strPtr("GetEvent"),
						InputType:  strPtr(".test.multi.GetEventRequest"),
						OutputType: strPtr(".test.multi.Event"),
					},
				},
			},
		},
	}

	files, err := buildFileDescriptors([]*descriptorpb.FileDescriptorProto{svcFDP}, discardLogger)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "test.multi.EventService")
	if sd == nil {
		t.Fatal("expected to find EventService")
	}
}

func TestBuildFileDescriptors_FourFilesWithCrossRefs(t *testing.T) {
	// Tests 4 files with cross-file type references:
	// 1. google_protobuf.proto (non-canonical, defines Timestamp+Duration)
	// 2. test_types.proto (custom types: Money)
	// 3. test_common.proto (uses Timestamp, imports google_protobuf.proto)
	// 4. test_svc.proto (service, uses Timestamp + cross-file types, all imports declared)
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	int32Type := descriptorpb.FieldDescriptorProto_TYPE_INT32
	int64Type := descriptorpb.FieldDescriptorProto_TYPE_INT64
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	// 1. Non-canonical barrel file defining google.protobuf.Timestamp + Duration
	wktFDP := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("google_protobuf.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("google.protobuf"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Timestamp"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("seconds"), Number: int32Ptr(1), Type: &int64Type, Label: &label},
					{Name: strPtr("nanos"), Number: int32Ptr(2), Type: &int32Type, Label: &label},
				},
			},
			{
				Name: strPtr("Duration"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("seconds"), Number: int32Ptr(1), Type: &int64Type, Label: &label},
					{Name: strPtr("nanos"), Number: int32Ptr(2), Type: &int32Type, Label: &label},
				},
			},
		},
	}

	// 2. Custom type file
	typeFDP := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test_types.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("test.types"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Money"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("amount"), Number: int32Ptr(1), Type: &int64Type, Label: &label},
					{Name: strPtr("currency"), Number: int32Ptr(2), Type: &strType, Label: &label},
				},
			},
		},
	}

	// 3. Shared types file that uses Timestamp
	commonFDP := &descriptorpb.FileDescriptorProto{
		Name:       strPtr("test_common.proto"),
		Syntax:     strPtr("proto3"),
		Package:    strPtr("test.common"),
		Dependency: []string{"google_protobuf.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("DateValue"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("value"), Number: int32Ptr(1), Type: &msgType, Label: &label, TypeName: strPtr(".google.protobuf.Timestamp")},
				},
			},
		},
	}

	// 4. Service file with all imports properly declared
	svcFDP := &descriptorpb.FileDescriptorProto{
		Name:       strPtr("test_svc.proto"),
		Syntax:     strPtr("proto3"),
		Package:    strPtr("test.noncanonical.v1"),
		Dependency: []string{"google_protobuf.proto", "test_common.proto", "test_types.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Item"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &strType, Label: &label},
					{Name: strPtr("created_at"), Number: int32Ptr(2), Type: &msgType, Label: &label, TypeName: strPtr(".google.protobuf.Timestamp")},
					{Name: strPtr("date"), Number: int32Ptr(3), Type: &msgType, Label: &label, TypeName: strPtr(".test.common.DateValue")},
					{Name: strPtr("price"), Number: int32Ptr(4), Type: &msgType, Label: &label, TypeName: strPtr(".test.types.Money")},
				},
			},
			{
				Name: strPtr("GetItemRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &strType, Label: &label},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: strPtr("NonCanonicalService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       strPtr("GetItem"),
						InputType:  strPtr(".test.noncanonical.v1.GetItemRequest"),
						OutputType: strPtr(".test.noncanonical.v1.Item"),
					},
				},
			},
		},
	}

	// Test with service first (deps not yet built)
	t.Run("ServiceFirst", func(t *testing.T) {
		files, err := buildFileDescriptors(
			[]*descriptorpb.FileDescriptorProto{svcFDP, wktFDP, commonFDP, typeFDP},
			discardLogger,
		)
		if err != nil {
			t.Fatalf("buildFileDescriptors failed: %v", err)
		}
		sd := findService(files, "test.noncanonical.v1.NonCanonicalService")
		if sd == nil {
			t.Fatal("expected to find NonCanonicalService")
		}
	})

	// Test with barrel file first (builds into localFiles before service)
	t.Run("BarrelFirst", func(t *testing.T) {
		files, err := buildFileDescriptors(
			[]*descriptorpb.FileDescriptorProto{wktFDP, typeFDP, commonFDP, svcFDP},
			discardLogger,
		)
		if err != nil {
			t.Fatalf("buildFileDescriptors failed: %v", err)
		}
		sd := findService(files, "test.noncanonical.v1.NonCanonicalService")
		if sd == nil {
			t.Fatal("expected to find NonCanonicalService")
		}
	})
}

func TestBuildFileDescriptors_DependencyOrdering(t *testing.T) {
	// Two custom files where A depends on B. Provide them in wrong order.
	// Build loop should handle ordering iteratively.
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	commonFDP := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("common.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("test.common"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Pagination"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("page"), Number: int32Ptr(1), Type: &strType, Label: &label},
				},
			},
		},
	}

	svcFDP := &descriptorpb.FileDescriptorProto{
		Name:       strPtr("service.proto"),
		Syntax:     strPtr("proto3"),
		Package:    strPtr("test.svc"),
		Dependency: []string{"common.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("ListRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("pagination"), Number: int32Ptr(1), Type: &msgType, Label: &label, TypeName: strPtr(".test.common.Pagination")},
				},
			},
			{
				Name: strPtr("ListResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("items"), Number: int32Ptr(1), Type: &strType, Label: &label},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: strPtr("ListService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       strPtr("List"),
						InputType:  strPtr(".test.svc.ListRequest"),
						OutputType: strPtr(".test.svc.ListResponse"),
					},
				},
			},
		},
	}

	// Provide in wrong order: service before common
	files, err := buildFileDescriptors([]*descriptorpb.FileDescriptorProto{svcFDP, commonFDP}, discardLogger)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "test.svc.ListService")
	if sd == nil {
		t.Fatal("expected to find ListService")
	}
}

// --- fixMapEntryNames unit tests ---

func TestFixMapEntryNames_FixesIncorrectName(t *testing.T) {
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test.pkg"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("MyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("competitions"),
						Number:   int32Ptr(1),
						Type:     &msgType,
						TypeName: strPtr(".test.pkg.MyMessage.CompetitionEntry"), // WRONG
						Label:    &labelRep,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("CompetitionEntry"), // WRONG: should be CompetitionsEntry
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
						},
					},
				},
			},
		},
	}

	fixed := fixMapEntryNames(fd)
	if !fixed {
		t.Fatal("expected fixMapEntryNames to return true")
	}

	entry := fd.GetMessageType()[0].GetNestedType()[0]
	if entry.GetName() != "CompetitionsEntry" {
		t.Errorf("expected entry name CompetitionsEntry, got %s", entry.GetName())
	}

	field := fd.GetMessageType()[0].GetField()[0]
	if field.GetTypeName() != ".test.pkg.MyMessage.CompetitionsEntry" {
		t.Errorf("expected field type .test.pkg.MyMessage.CompetitionsEntry, got %s", field.GetTypeName())
	}
}

func TestFixMapEntryNames_LeavesCorrectNameAlone(t *testing.T) {
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test.pkg"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("MyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("metadata"),
						Number:   int32Ptr(1),
						Type:     &msgType,
						TypeName: strPtr(".test.pkg.MyMessage.MetadataEntry"),
						Label:    &labelRep,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("MetadataEntry"), // Already correct
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
						},
					},
				},
			},
		},
	}

	fixed := fixMapEntryNames(fd)
	if fixed {
		t.Error("expected fixMapEntryNames to return false for correct map entry")
	}
}

func TestFixMapEntryNames_SnakeCaseField(t *testing.T) {
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test.pkg"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Response"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("events_by_org"),
						Number:   int32Ptr(1),
						Type:     &msgType,
						TypeName: strPtr(".test.pkg.Response.EventByOrgEntry"), // WRONG
						Label:    &labelRep,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("EventByOrgEntry"), // WRONG: should be EventsByOrgEntry
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
						},
					},
				},
			},
		},
	}

	fixed := fixMapEntryNames(fd)
	if !fixed {
		t.Fatal("expected fixMapEntryNames to return true")
	}

	entry := fd.GetMessageType()[0].GetNestedType()[0]
	if entry.GetName() != "EventsByOrgEntry" {
		t.Errorf("expected EventsByOrgEntry, got %s", entry.GetName())
	}
}

func TestMapEntryName(t *testing.T) {
	tests := []struct {
		field    string
		expected string
	}{
		{"metadata", "MetadataEntry"},
		{"competitions", "CompetitionsEntry"},
		{"events_by_org", "EventsByOrgEntry"},
		{"key_values", "KeyValuesEntry"},
		{"foo_bar_baz", "FooBarBazEntry"},
		{"a", "AEntry"},
	}
	for _, tt := range tests {
		got := mapEntryName(tt.field)
		if got != tt.expected {
			t.Errorf("mapEntryName(%q) = %q, want %q", tt.field, got, tt.expected)
		}
	}
}

// --- fixMapEntryNames: relative TypeName tests ---

func TestFixMapEntryNames_RelativeTypeNames(t *testing.T) {
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	t.Run("BareRelative", func(t *testing.T) {
		// TypeName is just the entry name with no package/message prefix.
		fd := &descriptorpb.FileDescriptorProto{
			Name:    strPtr("test.proto"),
			Package: strPtr("test.pkg"),
			MessageType: []*descriptorpb.DescriptorProto{
				{
					Name: strPtr("MyResponse"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:     strPtr("items"),
							Number:   int32Ptr(1),
							Type:     &msgType,
							TypeName: strPtr("Items"), // bare relative
							Label:    &labelRep,
						},
					},
					NestedType: []*descriptorpb.DescriptorProto{
						{
							Name:    strPtr("Items"), // wrong: should be ItemsEntry
							Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
							Field: []*descriptorpb.FieldDescriptorProto{
								{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
								{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
							},
						},
					},
				},
			},
		}

		fixed := fixMapEntryNames(fd)
		if !fixed {
			t.Fatal("expected fixMapEntryNames to return true")
		}

		entry := fd.GetMessageType()[0].GetNestedType()[0]
		if entry.GetName() != "ItemsEntry" {
			t.Errorf("expected entry name ItemsEntry, got %s", entry.GetName())
		}

		field := fd.GetMessageType()[0].GetField()[0]
		if field.GetTypeName() != "ItemsEntry" {
			t.Errorf("expected field type ItemsEntry, got %s", field.GetTypeName())
		}
	})

	t.Run("SuffixRelative", func(t *testing.T) {
		// TypeName uses parent message prefix: "MyResponse.Items"
		fd := &descriptorpb.FileDescriptorProto{
			Name:    strPtr("test.proto"),
			Package: strPtr("test.pkg"),
			MessageType: []*descriptorpb.DescriptorProto{
				{
					Name: strPtr("MyResponse"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:     strPtr("items"),
							Number:   int32Ptr(1),
							Type:     &msgType,
							TypeName: strPtr("MyResponse.Items"), // suffix relative
							Label:    &labelRep,
						},
					},
					NestedType: []*descriptorpb.DescriptorProto{
						{
							Name:    strPtr("Items"), // wrong: should be ItemsEntry
							Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
							Field: []*descriptorpb.FieldDescriptorProto{
								{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
								{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
							},
						},
					},
				},
			},
		}

		fixed := fixMapEntryNames(fd)
		if !fixed {
			t.Fatal("expected fixMapEntryNames to return true")
		}

		entry := fd.GetMessageType()[0].GetNestedType()[0]
		if entry.GetName() != "ItemsEntry" {
			t.Errorf("expected entry name ItemsEntry, got %s", entry.GetName())
		}

		field := fd.GetMessageType()[0].GetField()[0]
		if field.GetTypeName() != "MyResponse.ItemsEntry" {
			t.Errorf("expected field type MyResponse.ItemsEntry, got %s", field.GetTypeName())
		}
	})
}

func TestFixMapEntryNames_MixedTypeNames(t *testing.T) {
	// A single message with three map fields using different TypeName styles:
	// absolute, bare relative, and suffix relative.
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test.pkg"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Msg"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("items_by_id"),
						Number:   int32Ptr(1),
						Type:     &msgType,
						TypeName: strPtr(".test.pkg.Msg.ItemById"), // absolute, wrong
						Label:    &labelRep,
					},
					{
						Name:     strPtr("labels"),
						Number:   int32Ptr(2),
						Type:     &msgType,
						TypeName: strPtr("Label"), // bare relative, wrong
						Label:    &labelRep,
					},
					{
						Name:     strPtr("scores"),
						Number:   int32Ptr(3),
						Type:     &msgType,
						TypeName: strPtr("Msg.Score"), // suffix relative, wrong
						Label:    &labelRep,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("ItemById"), // wrong: should be ItemsByIdEntry
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
						},
					},
					{
						Name:    strPtr("Label"), // wrong: should be LabelsEntry
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
						},
					},
					{
						Name:    strPtr("Score"), // wrong: should be ScoresEntry
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
						},
					},
				},
			},
		},
	}

	fixed := fixMapEntryNames(fd)
	if !fixed {
		t.Fatal("expected fixMapEntryNames to return true")
	}

	msg := fd.GetMessageType()[0]

	// Absolute ref: renamed and TypeName updated with full path
	if msg.GetNestedType()[0].GetName() != "ItemsByIdEntry" {
		t.Errorf("expected ItemsByIdEntry, got %s", msg.GetNestedType()[0].GetName())
	}
	if msg.GetField()[0].GetTypeName() != ".test.pkg.Msg.ItemsByIdEntry" {
		t.Errorf("expected .test.pkg.Msg.ItemsByIdEntry, got %s", msg.GetField()[0].GetTypeName())
	}

	// Bare relative ref: renamed and TypeName replaced
	if msg.GetNestedType()[1].GetName() != "LabelsEntry" {
		t.Errorf("expected LabelsEntry, got %s", msg.GetNestedType()[1].GetName())
	}
	if msg.GetField()[1].GetTypeName() != "LabelsEntry" {
		t.Errorf("expected LabelsEntry, got %s", msg.GetField()[1].GetTypeName())
	}

	// Suffix relative ref: renamed and TypeName preserves prefix
	if msg.GetNestedType()[2].GetName() != "ScoresEntry" {
		t.Errorf("expected ScoresEntry, got %s", msg.GetNestedType()[2].GetName())
	}
	if msg.GetField()[2].GetTypeName() != "Msg.ScoresEntry" {
		t.Errorf("expected Msg.ScoresEntry, got %s", msg.GetField()[2].GetTypeName())
	}
}

// --- fixReservedRanges unit tests ---

func TestFixReservedRanges_FixesInvalidRange(t *testing.T) {
	start := int32(2)
	end := int32(2) // invalid: end == start means empty range
	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("MyMessage"),
				ReservedRange: []*descriptorpb.DescriptorProto_ReservedRange{
					{Start: &start, End: &end},
				},
			},
		},
	}
	if !fixReservedRanges(fd) {
		t.Fatal("expected fixReservedRanges to return true")
	}
	got := fd.GetMessageType()[0].GetReservedRange()[0].GetEnd()
	if got != 3 {
		t.Errorf("expected end=3, got %d", got)
	}
}

func TestFixReservedRanges_LeavesValidRangeAlone(t *testing.T) {
	start := int32(5)
	end := int32(6) // valid: [5, 6)
	fd := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("test.proto"),
		Package: strPtr("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("MyMessage"),
				ReservedRange: []*descriptorpb.DescriptorProto_ReservedRange{
					{Start: &start, End: &end},
				},
			},
		},
	}
	if fixReservedRanges(fd) {
		t.Fatal("expected fixReservedRanges to return false for valid range")
	}
}

// --- resolveTypeRef unit tests ---

func TestResolveTypeRef(t *testing.T) {
	// Build a small local registry with custom.types.Money
	int64Type := descriptorpb.FieldDescriptorProto_TYPE_INT64
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	moneyFDP := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("custom_types.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("custom.types"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Money"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("amount"), Number: int32Ptr(1), Type: &int64Type, Label: &labelOpt},
					{Name: strPtr("currency"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
				},
			},
		},
	}

	opts := protodesc.FileOptions{AllowUnresolvable: true}
	moneyFD, err := opts.New(moneyFDP, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatalf("failed to build money file: %v", err)
	}

	localFiles := new(protoregistry.Files)
	if err := localFiles.RegisterFile(moneyFD); err != nil {
		t.Fatalf("failed to register money file: %v", err)
	}

	resolver := &combinedResolver{local: localFiles, global: protoregistry.GlobalFiles}

	t.Run("FullyQualifiedWKT", func(t *testing.T) {
		// "google.protobuf.Timestamp" matches as-is from GlobalFiles
		d := resolveTypeRef("google.protobuf.Timestamp", "custom.event.v1", resolver)
		if d == nil {
			t.Fatal("expected to resolve google.protobuf.Timestamp")
		}
		if string(d.FullName()) != "google.protobuf.Timestamp" {
			t.Errorf("expected google.protobuf.Timestamp, got %s", d.FullName())
		}
	})

	t.Run("RelativeCrossFile", func(t *testing.T) {
		// "types.Money" in package "custom.event.v1" should resolve:
		// custom.event.v1.types.Money → no
		// custom.event.types.Money → no
		// custom.types.Money → found!
		d := resolveTypeRef("types.Money", "custom.event.v1", resolver)
		if d == nil {
			t.Fatal("expected to resolve types.Money from custom.event.v1")
		}
		if string(d.FullName()) != "custom.types.Money" {
			t.Errorf("expected custom.types.Money, got %s", d.FullName())
		}
	})

	t.Run("RelativeFromNonMatchingPackage", func(t *testing.T) {
		// "types.Money" in package "other.service.v1" cannot resolve:
		// other.service.v1.types.Money → no
		// other.service.types.Money → no
		// other.types.Money → no
		d := resolveTypeRef("types.Money", "other.service.v1", resolver)
		if d != nil {
			t.Errorf("expected nil, got %s", d.FullName())
		}
	})

	t.Run("UnresolvableRef", func(t *testing.T) {
		d := resolveTypeRef("nonexistent.Widget", "custom.event.v1", resolver)
		if d != nil {
			t.Errorf("expected nil for nonexistent type, got %s", d.FullName())
		}
	})
}

// --- Full scenario test: map entries + missing imports + non-canonical WKT ---

func TestBuildFileDescriptors_FullNonCanonicalScenario(t *testing.T) {
	// Tests all three fix-ups together: non-canonical WKT barrel file,
	// malformed map entries, empty dependencies, cross-file type refs.
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	int32Type := descriptorpb.FieldDescriptorProto_TYPE_INT32
	int64Type := descriptorpb.FieldDescriptorProto_TYPE_INT64
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	// Non-canonical barrel file
	wktFDP := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("google_protobuf.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("google.protobuf"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Timestamp"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("seconds"), Number: int32Ptr(1), Type: &int64Type, Label: &labelOpt},
					{Name: strPtr("nanos"), Number: int32Ptr(2), Type: &int32Type, Label: &labelOpt},
				},
			},
		},
	}

	// Custom types file
	typesFDP := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("custom_types.proto"),
		Syntax:  strPtr("proto3"),
		Package: strPtr("custom.types"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Money"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("amount"), Number: int32Ptr(1), Type: &int64Type, Label: &labelOpt},
					{Name: strPtr("currency"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
				},
			},
		},
	}

	// Service file with:
	// 1. NO dependencies declared (empty)
	// 2. Uses Timestamp from non-canonical barrel
	// 3. Uses Money from custom_types.proto
	// 4. Has a MALFORMED map entry (ItemEntry instead of ItemsEntry)
	svcFDP := &descriptorpb.FileDescriptorProto{
		Name:       strPtr("noncanonical_svc.proto"),
		Syntax:     strPtr("proto3"),
		Package:    strPtr("test.noncanonical.v1"),
		Dependency: []string{}, // EMPTY!
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Widget"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("name"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
					{Name: strPtr("created_at"), Number: int32Ptr(2), Type: &msgType, Label: &labelOpt, TypeName: strPtr(".google.protobuf.Timestamp")},
					{Name: strPtr("price"), Number: int32Ptr(3), Type: &msgType, Label: &labelOpt, TypeName: strPtr(".custom.types.Money")},
				},
			},
			{
				Name: strPtr("GetItemRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
				},
			},
			{
				Name: strPtr("ListItemsResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("items"),
						Number:   int32Ptr(1),
						Type:     &msgType,
						TypeName: strPtr(".test.noncanonical.v1.ListItemsResponse.ItemEntry"), // WRONG
						Label:    &labelRep,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("ItemEntry"), // WRONG: should be ItemsEntry
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &msgType, Label: &labelOpt, TypeName: strPtr(".test.noncanonical.v1.Widget")},
						},
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: strPtr("NonCanonicalService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       strPtr("GetItem"),
						InputType:  strPtr(".test.noncanonical.v1.GetItemRequest"),
						OutputType: strPtr(".test.noncanonical.v1.Widget"),
					},
				},
			},
		},
	}

	files, err := buildFileDescriptors(
		[]*descriptorpb.FileDescriptorProto{svcFDP, wktFDP, typesFDP},
		discardLogger,
	)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "test.noncanonical.v1.NonCanonicalService")
	if sd == nil {
		t.Fatal("expected to find NonCanonicalService")
	}
	if sd.Methods().Len() != 1 {
		t.Errorf("expected 1 method, got %d", sd.Methods().Len())
	}
}

// --- buildFileDescriptors: relative TypeName integration test ---

func TestBuildFileDescriptors_RelativeTypeNames(t *testing.T) {
	// Integration test mimicking real-world non-canonical servers where:
	// - ALL files have 0 declared dependencies
	// - TypeNames are relative (no leading dots)
	// - Map entries drop the "Entry" suffix entirely
	msgType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	int64Type := descriptorpb.FieldDescriptorProto_TYPE_INT64
	labelOpt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	labelRep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	// Custom types file - no deps
	typesFDP := &descriptorpb.FileDescriptorProto{
		Name:       strPtr("custom_types.proto"),
		Syntax:     strPtr("proto3"),
		Package:    strPtr("custom.types"),
		Dependency: []string{},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Money"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("amount"), Number: int32Ptr(1), Type: &int64Type, Label: &labelOpt},
					{Name: strPtr("currency"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
				},
			},
		},
	}

	// Service file with ALL non-canonical patterns:
	// - 0 declared dependencies
	// - Relative WKT ref: "google.protobuf.Timestamp" (no leading dot)
	// - Relative cross-file ref: "types.Money" (no leading dot)
	// - Same-file ref: "Event" (no leading dot)
	// - Map entry with "Entry" suffix dropped: "ItemsById" instead of "ItemsByIdEntry"
	svcFDP := &descriptorpb.FileDescriptorProto{
		Name:       strPtr("event_service.proto"),
		Syntax:     strPtr("proto3"),
		Package:    strPtr("custom.event.v1"),
		Dependency: []string{}, // EMPTY
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("Event"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("name"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
					{Name: strPtr("created_at"), Number: int32Ptr(2), Type: &msgType, Label: &labelOpt,
						TypeName: strPtr("google.protobuf.Timestamp")}, // relative WKT
					{Name: strPtr("price"), Number: int32Ptr(3), Type: &msgType, Label: &labelOpt,
						TypeName: strPtr("types.Money")}, // relative cross-file
					{
						Name:     strPtr("items_by_id"),
						Number:   int32Ptr(4),
						Type:     &msgType,
						TypeName: strPtr("ItemsById"), // bare relative, no "Entry" suffix
						Label:    &labelRep,
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("ItemsById"), // wrong: should be ItemsByIdEntry
						Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: strPtr("key"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
							{Name: strPtr("value"), Number: int32Ptr(2), Type: &strType, Label: &labelOpt},
						},
					},
				},
			},
			{
				Name: strPtr("GetEventRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("id"), Number: int32Ptr(1), Type: &strType, Label: &labelOpt},
				},
			},
			{
				Name: strPtr("GetEventResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strPtr("event"), Number: int32Ptr(1), Type: &msgType, Label: &labelOpt,
						TypeName: strPtr("Event")}, // same-file relative
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
						OutputType: strPtr(".custom.event.v1.GetEventResponse"),
					},
				},
			},
		},
	}

	files, err := buildFileDescriptors(
		[]*descriptorpb.FileDescriptorProto{svcFDP, typesFDP},
		discardLogger,
	)
	if err != nil {
		t.Fatalf("buildFileDescriptors failed: %v", err)
	}

	sd := findService(files, "custom.event.v1.EventService")
	if sd == nil {
		t.Fatal("expected to find custom.event.v1.EventService")
	}
	if sd.Methods().Len() != 1 {
		t.Errorf("expected 1 method, got %d", sd.Methods().Len())
	}
}

// --- Integration tests against testdata/noncanonical server ---

func TestIntegration_NonCanonicalServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Resolve paths relative to the test file's package directory
	serverDir, err := filepath.Abs("../../testdata/noncanonical")
	if err != nil {
		t.Fatalf("failed to resolve server dir: %v", err)
	}
	serverBin := filepath.Join(serverDir, "noncanonical")

	buildCmd := exec.Command("go", "build", "-o", serverBin, ".")
	buildCmd.Dir = serverDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build noncanonical server: %v\n%s", err, out)
	}
	defer os.Remove(serverBin)

	// Find a free port
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	addr := fmt.Sprintf("localhost:%d", port)

	// Start the server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, serverBin, "-addr", addr)
	cmd.Dir = serverDir
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start noncanonical server: %v", err)
	}
	defer func() {
		cancel()
		cmd.Wait()
	}()

	// Wait for the server to accept connections
	var conn *googlegrpc.ClientConn
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		conn, err = googlegrpc.NewClient(addr, googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			continue
		}
		// Verify the connection is actually usable
		_, connCancel := context.WithTimeout(ctx, 200*time.Millisecond)
		conn.Connect()
		state := conn.GetState()
		connCancel()
		if state == 2 { // connectivity.Ready
			break
		}
		conn.Close()
		conn = nil
	}
	if conn == nil {
		// Just create the connection; ListServices will surface the actual error
		conn, _ = googlegrpc.NewClient(addr, googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	defer conn.Close()

	// Give server a moment to be fully ready
	time.Sleep(500 * time.Millisecond)

	// Create a reflection client with a verbose logger for debugging
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	reflClient := NewReflectionClient(conn, logger)

	// ListServices should discover services and resolve them via lenientResolve
	services, err := reflClient.ListServices(ctx)
	if err != nil {
		t.Fatalf("ListServices failed: %v", err)
	}

	// Find our target service
	var eventSvc *domain.Service
	for i := range services {
		if services[i].FullName == "custom.event.v1.EventService" {
			eventSvc = &services[i]
			break
		}
	}

	if eventSvc == nil {
		t.Fatal("expected to find custom.event.v1.EventService in services")
	}

	if eventSvc.Error != "" {
		t.Errorf("expected no error for EventService, got:\n%s", eventSvc.Error)
	}

	if len(eventSvc.Methods) != 2 {
		t.Errorf("expected 2 methods (GetEvent, GetEvents), got %d", len(eventSvc.Methods))
	}
}

func boolPtr(b bool) *bool { return &b }
func strPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32 { return &i }
