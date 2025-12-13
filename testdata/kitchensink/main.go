package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	pb "github.com/shhac/grotto/testdata/kitchensink/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// kitchenSinkServer implements the KitchenSink service
type kitchenSinkServer struct {
	pb.UnimplementedKitchenSinkServer
	tasks map[string]*pb.Task
}

func newKitchenSinkServer() *kitchenSinkServer {
	return &kitchenSinkServer{
		tasks: make(map[string]*pb.Task),
	}
}

// UpsertTask creates or updates a task
func (s *kitchenSinkServer) UpsertTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	if req.Task == nil {
		return &pb.TaskResponse{
			Success: false,
			Message: "Task is required",
		}, nil
	}

	task := req.Task

	// Generate ID if not provided
	if task.Id == "" {
		task.Id = fmt.Sprintf("task-%d", time.Now().Unix())
	}

	// Set created_at if not set
	if task.CreatedAt == nil {
		task.CreatedAt = timestamppb.Now()
	}

	// Echo back with modifications: add a metadata entry
	if task.Metadata == nil {
		task.Metadata = make(map[string]string)
	}
	task.Metadata["last_modified"] = time.Now().Format(time.RFC3339)
	task.Metadata["server"] = "kitchen-sink-test-server"

	// Store the task
	s.tasks[task.Id] = task

	return &pb.TaskResponse{
		Task:    task,
		Message: fmt.Sprintf("Task %s upserted successfully", task.Id),
		Success: true,
	}, nil
}

// GetTask retrieves a task by ID
func (s *kitchenSinkServer) GetTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	if req.Task == nil || req.Task.Id == "" {
		return &pb.TaskResponse{
			Success: false,
			Message: "Task ID is required",
		}, nil
	}

	task, ok := s.tasks[req.Task.Id]
	if !ok {
		return &pb.TaskResponse{
			Success: false,
			Message: fmt.Sprintf("Task %s not found", req.Task.Id),
		}, nil
	}

	return &pb.TaskResponse{
		Task:    task,
		Message: "Task retrieved successfully",
		Success: true,
	}, nil
}

// ListTasks lists tasks with optional filtering
func (s *kitchenSinkServer) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	var tasks []*pb.Task

	for _, task := range s.tasks {
		// Apply filters
		if req.PriorityFilter != nil && task.Priority != *req.PriorityFilter {
			continue
		}
		if req.StatusFilter != nil && task.Status != *req.StatusFilter {
			continue
		}
		if req.AssigneeEmail != nil && (task.Assignee == nil || task.Assignee.Email != *req.AssigneeEmail) {
			continue
		}

		tasks = append(tasks, task)
	}

	// Simple pagination (not implementing page_token for this test server)
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	if len(tasks) > pageSize {
		tasks = tasks[:pageSize]
	}

	return &pb.ListTasksResponse{
		Tasks:      tasks,
		TotalCount: int32(len(tasks)),
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", "localhost:50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()

	// Register kitchen sink service
	pb.RegisterKitchenSinkServer(s, newKitchenSinkServer())

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Enable reflection for grpcurl/grpcui
	reflection.Register(s)

	log.Printf("Kitchen Sink gRPC test server listening on localhost:50052")
	log.Printf("Services: kitchensink.KitchenSink, grpc.health.v1.Health")
	log.Printf("Reflection enabled")
	log.Println("\nExample usage:")
	log.Println("  grpcurl -plaintext localhost:50052 list")
	log.Println("  grpcurl -plaintext localhost:50052 describe kitchensink.KitchenSink")
	log.Println("  grpcui -plaintext localhost:50052")

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
