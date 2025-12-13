package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Simple greeter service for testing
type greeterServer struct {
	UnimplementedGreeterServer
}

type UnimplementedGreeterServer struct{}

func (s *greeterServer) SayHello(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
	return &HelloReply{
		Message: "Hello " + req.Name,
	}, nil
}

type HelloRequest struct {
	Name string
}

type HelloReply struct {
	Message string
}

func main() {
	lis, err := net.Listen("tcp", "localhost:50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Enable reflection
	reflection.Register(s)

	log.Printf("gRPC test server listening on localhost:50051")
	log.Printf("Services: grpc.health.v1.Health")
	log.Printf("Reflection enabled")

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
