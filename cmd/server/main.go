package main

import (
	"log"
	"net"
	pb "telegram-service/gen/telegram"
	"telegram-service/internal/config"
	"telegram-service/internal/service"
	"telegram-service/internal/session"

	"google.golang.org/grpc"
)

func main() {
	conf := config.MustLoad()

	mgr := session.NewManager(conf)
	srv := service.NewTelegramService(mgr)

	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}

	grpcSrv := grpc.NewServer(
		grpc.UnaryInterceptor(mgr.CheckSessionInterceptor),
	)
	pb.RegisterTelegramServiceServer(grpcSrv, srv)

	log.Println("gRPC server listening on :8080")
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
