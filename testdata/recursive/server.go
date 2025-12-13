package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/shhac/grotto/testdata/recursive/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// server implements the RecursiveService
type server struct {
	pb.UnimplementedRecursiveServiceServer
}

// EchoTree echoes the tree node back
func (s *server) EchoTree(ctx context.Context, req *pb.TreeNode) (*pb.TreeNode, error) {
	log.Printf("EchoTree called with value: %d", req.Value)
	return req, nil
}

// EchoLinkedList echoes the linked list node back
func (s *server) EchoLinkedList(ctx context.Context, req *pb.LinkedListNode) (*pb.LinkedListNode, error) {
	log.Printf("EchoLinkedList called with data: %s", req.Data)
	return req, nil
}

// EchoPerson echoes the person back
func (s *server) EchoPerson(ctx context.Context, req *pb.Person) (*pb.Person, error) {
	log.Printf("EchoPerson called with name: %s", req.Name)
	return req, nil
}

func StartServer() error {
	port := 50053
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s := grpc.NewServer()
	pb.RegisterRecursiveServiceServer(s, &server{})

	// Enable reflection for grpcurl and other tools
	reflection.Register(s)

	log.Printf("RecursiveService server listening on port %d", port)
	if err := s.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}
