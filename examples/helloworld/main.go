package main

import (
	"context"
	"log"
	"net"

	"github.com/blessli/piano"
	pb "github.com/blessli/piano/testdata"
)

var (
	port int = 8888
)

type greeterServiceImpl struct{}

func (s *greeterServiceImpl) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: %v", req.GetName())
	return &pb.HelloReply{Message: "Hello " + req.GetName()},nil
}

func main() {
	lis, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := piano.NewServer()
	pb.RegisterGreeterServer(s, &greeterServiceImpl{})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
