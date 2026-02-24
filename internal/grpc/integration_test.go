package grpc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ---------------------------------------------------------------------------
// Service Discovery Tests (grpcreflect via ReflectionClient)
// ---------------------------------------------------------------------------

func TestListServices(t *testing.T) {
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	services, err := rc.ListServices(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, services)

	// Our test service should appear.
	var found bool
	for _, svc := range services {
		if svc.FullName == "grpctest.TestService" {
			found = true
			assert.Equal(t, "TestService", svc.Name)
			assert.Len(t, svc.Methods, 4)
			break
		}
	}
	assert.True(t, found, "grpctest.TestService not found in listed services")
}

func TestListServices_SkipsReflection(t *testing.T) {
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	services, err := rc.ListServices(context.Background())
	require.NoError(t, err)

	for _, svc := range services {
		assert.NotContains(t, svc.FullName, "grpc.reflection",
			"reflection service should be filtered out")
	}
}

func TestResolveService_Methods(t *testing.T) {
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	services, err := rc.ListServices(context.Background())
	require.NoError(t, err)

	var methods []struct {
		name           string
		isClientStream bool
		isServerStream bool
	}
	for _, svc := range services {
		if svc.FullName == "grpctest.TestService" {
			for _, m := range svc.Methods {
				methods = append(methods, struct {
					name           string
					isClientStream bool
					isServerStream bool
				}{m.Name, m.IsClientStream, m.IsServerStream})
			}
		}
	}

	expected := map[string][2]bool{
		"UnaryEcho":    {false, false},
		"StreamItems":  {false, true},
		"CollectItems": {true, false},
		"BidiEcho":     {true, true},
	}

	require.Len(t, methods, len(expected))
	for _, m := range methods {
		exp, ok := expected[m.name]
		require.True(t, ok, "unexpected method: %s", m.name)
		assert.Equal(t, exp[0], m.isClientStream, "client stream mismatch for %s", m.name)
		assert.Equal(t, exp[1], m.isServerStream, "server stream mismatch for %s", m.name)
	}
}

func TestResolveService_FieldTypes(t *testing.T) {
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)

	inputType := md.Input()
	require.NotNil(t, inputType)
	assert.Equal(t, "grpctest.ItemRequest", string(inputType.FullName()))

	// ItemRequest has a single field "item" of type Item.
	itemField := inputType.Fields().ByName("item")
	require.NotNil(t, itemField, "field 'item' not found")

	itemMsg := itemField.Message()
	require.NotNil(t, itemMsg)
	assert.Equal(t, "grpctest.Item", string(itemMsg.FullName()))

	// Verify key field types exist on Item.
	fieldNames := []string{
		"id", "name", "color", "labels", "tags",
		"created_at", "ttl", "text", "number", "nested",
		"count", "active", "score", "data",
	}
	for _, name := range fieldNames {
		assert.NotNil(t, itemMsg.Fields().ByName(protoreflect.Name(name)), "field %q not found on Item", name)
	}

	// Verify map type.
	labelsField := itemMsg.Fields().ByName("labels")
	require.NotNil(t, labelsField)
	assert.True(t, labelsField.IsMap(), "labels should be a map field")

	// Verify repeated type.
	tagsField := itemMsg.Fields().ByName("tags")
	require.NotNil(t, tagsField)
	assert.True(t, tagsField.IsList(), "tags should be a repeated field")

	// Verify enum type.
	colorField := itemMsg.Fields().ByName("color")
	require.NotNil(t, colorField)
	assert.NotNil(t, colorField.Enum(), "color should be an enum field")

	// Verify oneof.
	textField := itemMsg.Fields().ByName("text")
	require.NotNil(t, textField)
	assert.NotNil(t, textField.ContainingOneof(), "text should be part of a oneof")
	numberField := itemMsg.Fields().ByName("number")
	require.NotNil(t, numberField)
	assert.Equal(t, textField.ContainingOneof(), numberField.ContainingOneof(),
		"text and number should belong to the same oneof")

	// Verify nested message type.
	nestedField := itemMsg.Fields().ByName("nested")
	require.NotNil(t, nestedField)
	nestedMsg := nestedField.Message()
	require.NotNil(t, nestedMsg)
	assert.Equal(t, "grpctest.Nested", string(nestedMsg.FullName()))

	// Verify well-known types.
	createdAtField := itemMsg.Fields().ByName("created_at")
	require.NotNil(t, createdAtField)
	assert.Equal(t, "google.protobuf.Timestamp", string(createdAtField.Message().FullName()))

	ttlField := itemMsg.Fields().ByName("ttl")
	require.NotNil(t, ttlField)
	assert.Equal(t, "google.protobuf.Duration", string(ttlField.Message().FullName()))
}

func TestGetMethodDescriptor(t *testing.T) {
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	// First call resolves from server.
	md1, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)
	require.NotNil(t, md1)
	assert.Equal(t, protoreflect.Name("UnaryEcho"), md1.Name())

	// Second call should hit cache, same result.
	md2, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)
	assert.Equal(t, md1, md2, "cached descriptor should be identical")

	// Input and output types.
	assert.Equal(t, "grpctest.ItemRequest", string(md1.Input().FullName()))
	assert.Equal(t, "grpctest.ItemResponse", string(md1.Output().FullName()))
}

func TestGetMethodDescriptor_NotFound(t *testing.T) {
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	_, err := rc.GetMethodDescriptor("grpctest.TestService", "NoSuchMethod")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveService_NotFound(t *testing.T) {
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	_, err := rc.GetMethodDescriptor("nonexistent.Service", "Method")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// RPC Invocation Tests (grpcdynamic via Invoker)
// ---------------------------------------------------------------------------

func TestInvokeUnary(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)

	req := `{"item":{"id":"test-1","name":"hello","color":"RED","tags":["a","b"]}}`
	resp, _, _, err := inv.InvokeUnary(context.Background(), md, req, nil)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Equal(t, true, result["ok"])

	item, ok := result["item"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test-1", item["id"])
	assert.Equal(t, "hello", item["name"])
	assert.Equal(t, "RED", item["color"])
}

func TestInvokeUnary_EmptyRequest(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)

	resp, _, _, err := inv.InvokeUnary(context.Background(), md, `{}`, nil)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Equal(t, true, result["ok"])
}

func TestInvokeUnary_InvalidJSON(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)

	_, _, _, err = inv.InvokeUnary(context.Background(), md, `{invalid`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request JSON")
}

func TestInvokeServerStream(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "StreamItems")
	require.NoError(t, err)

	req := `{"item":{"id":"stream-1","name":"streamed"}}`
	msgChan, errChan, _, _ := inv.InvokeServerStream(context.Background(), md, req, nil)

	var messages []string
	for msg := range msgChan {
		messages = append(messages, msg)
	}

	// Should receive exactly 3 messages.
	assert.Len(t, messages, 3)

	// Stream should end with EOF.
	streamErr := <-errChan
	assert.Equal(t, io.EOF, streamErr)

	// Verify each message content.
	for _, msg := range messages {
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(msg), &result))
		assert.Equal(t, true, result["ok"])
		item := result["item"].(map[string]interface{})
		assert.Equal(t, "stream-1", item["id"])
	}
}

func TestInvokeServerStream_Cancel(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "StreamItems")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, errChan, _, _ := inv.InvokeServerStream(ctx, md, `{"item":{"id":"cancel"}}`, nil)

	streamErr := <-errChan
	require.Error(t, streamErr)
	assert.NotEqual(t, io.EOF, streamErr)
}

func TestInvokeClientStream(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "CollectItems")
	require.NoError(t, err)

	handle, err := inv.InvokeClientStream(context.Background(), md, nil)
	require.NoError(t, err)

	// Send 3 items.
	for i := 0; i < 3; i++ {
		req := `{"item":{"id":"item-` + strings.Repeat("x", i+1) + `"}}`
		require.NoError(t, handle.Send(req))
	}

	resp, err := handle.CloseAndReceive()
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Equal(t, float64(3), result["count"])

	items, ok := result["items"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 3)
}

func TestInvokeClientStream_EmptyStream(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "CollectItems")
	require.NoError(t, err)

	handle, err := inv.InvokeClientStream(context.Background(), md, nil)
	require.NoError(t, err)

	resp, err := handle.CloseAndReceive()
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp), &result))
	// Proto3 JSON omits zero-value int32, so count may be absent or 0.
	if count, ok := result["count"]; ok {
		assert.Equal(t, float64(0), count)
	}
	// No items should be present.
	_, hasItems := result["items"]
	assert.False(t, hasItems, "empty stream should have no items")
}

func TestInvokeBidiStream(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "BidiEcho")
	require.NoError(t, err)

	handle, err := inv.InvokeBidiStream(context.Background(), md, nil)
	require.NoError(t, err)

	// Interleaved send/recv.
	messages := []string{
		`{"item":{"id":"bidi-1","name":"first"}}`,
		`{"item":{"id":"bidi-2","name":"second"}}`,
		`{"item":{"id":"bidi-3","name":"third"}}`,
	}

	for _, msg := range messages {
		require.NoError(t, handle.Send(msg))

		resp, err := handle.Recv()
		require.NoError(t, err)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(resp), &result))
		assert.Equal(t, true, result["ok"])
	}

	require.NoError(t, handle.CloseSend())

	// After CloseSend, server should end the stream.
	_, err = handle.Recv()
	assert.Equal(t, io.EOF, err)
}

func TestInvokeBidiStream_CloseSendThenDrain(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "BidiEcho")
	require.NoError(t, err)

	handle, err := inv.InvokeBidiStream(context.Background(), md, nil)
	require.NoError(t, err)

	// Send two messages, then close send.
	require.NoError(t, handle.Send(`{"item":{"id":"d1"}}`))
	require.NoError(t, handle.Send(`{"item":{"id":"d2"}}`))
	require.NoError(t, handle.CloseSend())

	// Drain responses.
	var received int
	for {
		_, err := handle.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		received++
	}
	assert.Equal(t, 2, received)
}

// ---------------------------------------------------------------------------
// JSON Round-Trip Tests
// ---------------------------------------------------------------------------

func invokeUnaryJSON(t *testing.T, reqJSON string) map[string]interface{} {
	t.Helper()
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	md, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)

	resp, _, _, err := inv.InvokeUnary(context.Background(), md, reqJSON, nil)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp), &result))
	return result
}

func TestJSONRoundTrip_NestedMessages(t *testing.T) {
	result := invokeUnaryJSON(t, `{"item":{"nested":{"value":"deep"}}}`)
	item := result["item"].(map[string]interface{})
	nested := item["nested"].(map[string]interface{})
	assert.Equal(t, "deep", nested["value"])
}

func TestJSONRoundTrip_Maps(t *testing.T) {
	result := invokeUnaryJSON(t, `{"item":{"labels":{"env":"prod","region":"us-east"}}}`)
	item := result["item"].(map[string]interface{})
	labels := item["labels"].(map[string]interface{})
	assert.Equal(t, "prod", labels["env"])
	assert.Equal(t, "us-east", labels["region"])
}

func TestJSONRoundTrip_RepeatedFields(t *testing.T) {
	result := invokeUnaryJSON(t, `{"item":{"tags":["alpha","beta","gamma"]}}`)
	item := result["item"].(map[string]interface{})
	tags := item["tags"].([]interface{})
	assert.Len(t, tags, 3)
	assert.Equal(t, "alpha", tags[0])
	assert.Equal(t, "beta", tags[1])
	assert.Equal(t, "gamma", tags[2])
}

func TestJSONRoundTrip_Oneofs(t *testing.T) {
	// Set text variant.
	result := invokeUnaryJSON(t, `{"item":{"text":"hello"}}`)
	item := result["item"].(map[string]interface{})
	assert.Equal(t, "hello", item["text"])
	_, hasNumber := item["number"]
	assert.False(t, hasNumber, "number should not be present when text is set")

	// Set number variant.
	result = invokeUnaryJSON(t, `{"item":{"number":"42"}}`)
	item = result["item"].(map[string]interface{})
	// JSON numbers from proto int64 may be string-encoded.
	assert.NotNil(t, item["number"])
	_, hasText := item["text"]
	assert.False(t, hasText, "text should not be present when number is set")
}

func TestJSONRoundTrip_Enums(t *testing.T) {
	// Enum as string name.
	result := invokeUnaryJSON(t, `{"item":{"color":"GREEN"}}`)
	item := result["item"].(map[string]interface{})
	assert.Equal(t, "GREEN", item["color"])
}

func TestJSONRoundTrip_Enums_Numeric(t *testing.T) {
	// Enum as numeric value.
	result := invokeUnaryJSON(t, `{"item":{"color":2}}`)
	item := result["item"].(map[string]interface{})
	// Response should come back as string name.
	assert.Equal(t, "GREEN", item["color"])
}

func TestJSONRoundTrip_WellKnownTypes(t *testing.T) {
	req := `{"item":{` +
		`"createdAt":"2024-01-15T10:30:00Z",` +
		`"ttl":"3600s"` +
		`}}`
	result := invokeUnaryJSON(t, req)
	item := result["item"].(map[string]interface{})

	// Timestamp should roundtrip.
	createdAt, ok := item["createdAt"].(string)
	require.True(t, ok, "createdAt should be a string")
	ts, err := time.Parse(time.RFC3339, createdAt)
	require.NoError(t, err)
	assert.Equal(t, 2024, ts.Year())
	assert.Equal(t, time.January, ts.Month())
	assert.Equal(t, 15, ts.Day())

	// Duration should roundtrip.
	ttl, ok := item["ttl"].(string)
	require.True(t, ok, "ttl should be a string")
	assert.Contains(t, ttl, "3600")
}

func TestJSONRoundTrip_ScalarTypes(t *testing.T) {
	req := `{"item":{` +
		`"id":"scalar-test",` +
		`"count":42,` +
		`"active":true,` +
		`"score":3.14,` +
		`"data":"aGVsbG8="` +
		`}}`
	result := invokeUnaryJSON(t, req)
	item := result["item"].(map[string]interface{})

	assert.Equal(t, "scalar-test", item["id"])
	assert.Equal(t, float64(42), item["count"])
	assert.Equal(t, true, item["active"])
	assert.InDelta(t, 3.14, item["score"].(float64), 0.001)
	assert.Equal(t, "aGVsbG8=", item["data"]) // base64-encoded "hello"
}

func TestJSONRoundTrip_DefaultValues(t *testing.T) {
	// Empty item — proto3 zero values should be omitted from JSON.
	result := invokeUnaryJSON(t, `{"item":{}}`)
	item := result["item"].(map[string]interface{})

	// Zero-value fields should be absent in proto3 JSON.
	_, hasCount := item["count"]
	assert.False(t, hasCount, "zero count should be omitted")
	_, hasActive := item["active"]
	assert.False(t, hasActive, "false active should be omitted")
	_, hasScore := item["score"]
	assert.False(t, hasScore, "zero score should be omitted")
}

// ---------------------------------------------------------------------------
// Edge Case Tests
// ---------------------------------------------------------------------------

func TestEmptyMessage(t *testing.T) {
	result := invokeUnaryJSON(t, `{}`)
	assert.Equal(t, true, result["ok"])
}

func TestLargePayload(t *testing.T) {
	tags := make([]string, 500)
	for i := range tags {
		tags[i] = strings.Repeat("x", 10)
	}
	tagsJSON, _ := json.Marshal(tags)
	req := `{"item":{"tags":` + string(tagsJSON) + `}}`
	result := invokeUnaryJSON(t, req)
	item := result["item"].(map[string]interface{})
	resultTags := item["tags"].([]interface{})
	assert.Len(t, resultTags, 500)
}

func TestInvokeUnary_WithMetadata(t *testing.T) {
	inv := NewInvoker(testConn, testLogger)
	rc := NewReflectionClient(testConn, testLogger)
	defer rc.Close()

	methodDesc, err := rc.GetMethodDescriptor("grpctest.TestService", "UnaryEcho")
	require.NoError(t, err)

	md := metadata.New(map[string]string{
		"x-custom-header": "test-value",
	})

	resp, _, _, err := inv.InvokeUnary(context.Background(), methodDesc, `{"item":{"id":"meta"}}`, md)
	require.NoError(t, err)
	assert.NotEmpty(t, resp)
}

func TestJSONRoundTrip_ComplexItem(t *testing.T) {
	req := `{
		"item": {
			"id": "complex-1",
			"name": "Full Item",
			"color": "BLUE",
			"labels": {"tier": "premium", "version": "2.0"},
			"tags": ["important", "reviewed"],
			"createdAt": "2024-06-15T08:00:00Z",
			"ttl": "7200s",
			"text": "some payload",
			"nested": {"value": "inner"},
			"count": 99,
			"active": true,
			"score": 9.81
		}
	}`

	result := invokeUnaryJSON(t, req)
	item := result["item"].(map[string]interface{})

	assert.Equal(t, "complex-1", item["id"])
	assert.Equal(t, "Full Item", item["name"])
	assert.Equal(t, "BLUE", item["color"])
	assert.Equal(t, float64(99), item["count"])
	assert.Equal(t, true, item["active"])
	assert.InDelta(t, 9.81, item["score"].(float64), 0.001)
	assert.Equal(t, "some payload", item["text"])

	labels := item["labels"].(map[string]interface{})
	assert.Equal(t, "premium", labels["tier"])

	tags := item["tags"].([]interface{})
	assert.Len(t, tags, 2)

	nested := item["nested"].(map[string]interface{})
	assert.Equal(t, "inner", nested["value"])
}
