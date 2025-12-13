package main

import (
	"io"
	"log"
	"net"

	pb "github.com/shhac/grotto/testdata/bidistream/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type echoServer struct {
	pb.UnimplementedEchoServiceServer
}

func (s *echoServer) BidiEcho(stream pb.EchoService_BidiEchoServer) error {
	log.Println("BidiEcho stream started")

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			log.Println("BidiEcho stream ended (client closed)")
			return nil
		}
		if err != nil {
			log.Printf("BidiEcho error receiving: %v", err)
			return err
		}

		log.Printf("Received ping: %q", req.Ping)

		resp := &pb.PongResponse{
			Pong: req.Ping,
		}

		if err := stream.Send(resp); err != nil {
			log.Printf("BidiEcho error sending: %v", err)
			return err
		}

		log.Printf("Sent pong: %q", resp.Pong)
	}
}

func main() {
	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterEchoServiceServer(s, &echoServer{})

	// Enable reflection for grpcurl and similar tools
	reflection.Register(s)

	log.Println("BidiStream Echo Server listening on :50054")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
