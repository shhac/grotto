# Recursive gRPC Test Server

This test server demonstrates circular/self-referencing protobuf message types for testing Grotto's recursion handling.

## Proto Types

The `recursive.proto` file defines three circular/self-referencing types:

1. **TreeNode** - Direct self-reference with optional left/right children
   ```protobuf
   message TreeNode {
     int32 value = 1;
     optional TreeNode left = 2;
     optional TreeNode right = 3;
   }
   ```

2. **LinkedListNode** - Simple linear self-reference with optional next
   ```protobuf
   message LinkedListNode {
     string data = 1;
     optional LinkedListNode next = 2;
   }
   ```

3. **Person** - Indirect circular reference through repeated friends
   ```protobuf
   message Person {
     string name = 1;
     int32 age = 2;
     repeated Person friends = 3;
   }
   ```

## Running the Server

```bash
cd testdata/recursive
go run cmd/server/main.go
```

The server will listen on port 50053 with gRPC reflection enabled.

## Testing with grpcurl

List services:
```bash
grpcurl -plaintext localhost:50053 list
```

Test TreeNode:
```bash
grpcurl -plaintext -d '{"value": 1, "left": {"value": 2}, "right": {"value": 3}}' \
  localhost:50053 testdata.recursive.RecursiveService/EchoTree
```

Test LinkedListNode:
```bash
grpcurl -plaintext -d '{"data": "first", "next": {"data": "second", "next": {"data": "third"}}}' \
  localhost:50053 testdata.recursive.RecursiveService/EchoLinkedList
```

Test Person:
```bash
grpcurl -plaintext -d '{"name": "Alice", "age": 30, "friends": [{"name": "Bob", "age": 25}]}' \
  localhost:50053 testdata.recursive.RecursiveService/EchoPerson
```
