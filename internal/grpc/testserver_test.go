package grpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"

	pb "github.com/shhac/grotto/testdata/grpctest/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

// Package-level test infrastructure shared by all tests.
var (
	testConn   *grpc.ClientConn
	testServer *grpc.Server
	testLogger *slog.Logger
)

// testService is a trivial echo-back implementation of TestService.
type testService struct {
	pb.UnimplementedTestServiceServer
}

// UnaryEcho echoes the request item back with ok=true.
func (s *testService) UnaryEcho(_ context.Context, req *pb.ItemRequest) (*pb.ItemResponse, error) {
	return &pb.ItemResponse{
		Item: req.GetItem(),
		Ok:   true,
	}, nil
}

// StreamItems sends the request item back 3 times.
func (s *testService) StreamItems(req *pb.ItemRequest, stream pb.TestService_StreamItemsServer) error {
	for i := 0; i < 3; i++ {
		resp := &pb.ItemResponse{
			Item: req.GetItem(),
			Ok:   true,
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}

// CollectItems collects all sent items and returns an aggregated list.
func (s *testService) CollectItems(stream pb.TestService_CollectItemsServer) error {
	var items []*pb.Item
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.ItemList{
				Items: items,
				Count: int32(len(items)),
			})
		}
		if err != nil {
			return err
		}
		items = append(items, req.GetItem())
	}
}

// BidiEcho echoes each request immediately as a response.
func (s *testService) BidiEcho(stream pb.TestService_BidiEchoServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		resp := &pb.ItemResponse{
			Item: req.GetItem(),
			Ok:   true,
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func TestMain(m *testing.M) {
	// Listen on an ephemeral port.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen: %v\n", err)
		os.Exit(1)
	}

	// Create and start gRPC server with reflection.
	testServer = grpc.NewServer()
	pb.RegisterTestServiceServer(testServer, &testService{})
	reflection.Register(testServer)

	go func() {
		if err := testServer.Serve(lis); err != nil {
			fmt.Fprintf(os.Stderr, "server exited: %v\n", err)
		}
	}()

	// Create client connection.
	testConn, err = grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Use a nop logger for tests.
	testLogger = slog.New(slog.NewTextHandler(
		io.Discard,
		&slog.HandlerOptions{Level: slog.LevelError + 1},
	))

	code := m.Run()

	testConn.Close()
	testServer.Stop()
	os.Exit(code)
}
