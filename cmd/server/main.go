package main

import (
	"log"
	"net"

	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}

	grpcSrv := grpc.NewServer()

	log.Println("gRPC server listening on :8080")
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
