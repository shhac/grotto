# Kitchen Sink Test Server

A comprehensive gRPC test server for validating Grotto's form builder with complex nested objects.

## Features

The `kitchen_sink.proto` service includes:

- **Enums**: Priority (LOW, MEDIUM, HIGH, CRITICAL), Status (PENDING, IN_PROGRESS, COMPLETED, CANCELLED)
- **All scalar types**: int32, int64, uint32, uint64, sint32, sint64, fixed32, fixed64, sfixed32, sfixed64, float, double, bool, string, bytes
- **Optional fields**: Optional variants of all common scalar types
- **Nested messages**: Contact with Address, Task with Contact
- **Repeated fields**: Tags (repeated string), Watchers (repeated Contact)
- **Map fields**: Metadata (map<string, string>)
- **Well-known types**: Timestamp, Duration
- **Multiple RPC methods**: UpsertTask, GetTask, ListTasks

## Running the Server

```bash
# From the testdata/kitchensink directory
go run main.go
```

The server will start on `localhost:50052` with reflection enabled.

## Testing with grpcurl

```bash
# List services
grpcurl -plaintext localhost:50052 list

# Describe the KitchenSink service
grpcurl -plaintext localhost:50052 describe kitchensink.KitchenSink

# Describe a message type
grpcurl -plaintext localhost:50052 describe kitchensink.Task

# Call UpsertTask
grpcurl -plaintext -d '{
  "task": {
    "title": "Test Task",
    "description": "A comprehensive test",
    "priority": "HIGH",
    "status": "IN_PROGRESS",
    "tags": ["test", "grotto"],
    "assignee": {
      "name": "John Doe",
      "email": "john@example.com",
      "address": {
        "street": "123 Main St",
        "city": "San Francisco",
        "state": "CA",
        "zip_code": "94105",
        "country": "USA"
      }
    },
    "metadata": {
      "project": "grotto",
      "team": "engineering"
    }
  }
}' localhost:50052 kitchensink.KitchenSink/UpsertTask
```

## Testing with grpcui

For a web-based UI:

```bash
grpcui -plaintext localhost:50052
```

## Using with Grotto

Point Grotto to `localhost:50052` and explore the `kitchensink.KitchenSink.UpsertTask` method to see the comprehensive form builder in action.
