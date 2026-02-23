# Non-Canonical Protobuf Test Server

A gRPC test server that serves **malformed FileDescriptorProtos** via a custom
reflection handler. Used to test Grotto's resilience against real-world servers
that produce non-canonical protobuf descriptors.

## Malformations Reproduced

1. **Non-canonical WKT barrel file** — A file named `google_protobuf.proto`
   (instead of `google/protobuf/timestamp.proto` etc.) that defines both
   `Timestamp` and `Duration` in the `google.protobuf` package.

2. **Missing imports** — `event_service.proto` has an empty `Dependency` array
   despite referencing types from `google_protobuf.proto`, `custom_types.proto`,
   and `common.proto`.

3. **Malformed map entry names** — Map fields where the implicit entry message
   name doesn't follow the `CamelCase(field_name) + "Entry"` convention:
   - `key_values` field → `KeyValueEntry` (should be `KeyValuesEntry`)
   - `events_by_org` field → `EventByOrgEntry` (should be `EventsByOrgEntry`)

4. **Cross-file type references without imports** — Types defined in one file
   are referenced from another without the dependency being declared.

## Services

- `custom.event.v1.EventService` — Event management (reflection only, RPCs return errors)
- `grpc.health.v1.Health` — Standard health check

## Proto Files Served

| File | Package | Notes |
|------|---------|-------|
| `google_protobuf.proto` | `google.protobuf` | Non-canonical WKT barrel |
| `custom_types.proto` | `custom.types` | Standalone types |
| `common.proto` | `custom.common` | Malformed map entry name |
| `event_service.proto` | `custom.event.v1` | Missing imports, malformed map entry |

## Running

```bash
go run .
# Listens on localhost:50055
```

## Testing With

```bash
grpcurl -plaintext localhost:50055 list
grpcurl -plaintext localhost:50055 describe custom.event.v1.EventService
```
