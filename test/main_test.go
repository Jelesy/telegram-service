package test

import (
	"context"
	"log"
	"net"
	pb "telegram-service/gen/telegram"
	"telegram-service/internal/config"
	"telegram-service/internal/service"
	"telegram-service/internal/session"
	"testing"
	"time"

	"google.golang.org/grpc"
)

const (
	listenHost = "127.0.0.1"
	listenPort = "8082"
	listenAddr = listenHost + ":" + listenPort
)

// Ожидание в десятках миллисекунд
// amount * 10ms
func wait(amount int) {
	time.Sleep(time.Duration(amount) * 10 * time.Millisecond)
}

func StartMyGrpcServer(ctx context.Context, listenAddr string) error {
	cfg := config.MustLoad("../.env")

	mgr := session.NewManager(cfg)
	srv := service.NewTelegramService(mgr)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(mgr.CheckSessionInterceptor),
	)
	pb.RegisterTelegramServiceServer(grpcServer, srv)

	// microservice block
	go func() {
		stop := context.AfterFunc(ctx, func() {
			grpcServer.GracefulStop()
		})
		defer stop()

		log.Println("gRPC сервер запускается по адресу", listenAddr)
		err := grpcServer.Serve(lis)
		if err != nil {
			log.Fatalf("Ошибка запуска gRPC сервера: %v", err)
		}
	}()
	time.Sleep(15 * time.Millisecond)

	return nil
}

// старт-стоп сервера
func TestServerStartStop(t *testing.T) {
	ctx, finish := context.WithCancel(context.Background())
	err := StartMyGrpcServer(ctx, listenAddr)
	if err != nil {
		t.Fatalf("cant start server initial: %v", err)
	}
	wait(1)
	finish() // при вызове этой функции ваш сервер должен остановиться и освободить порт
	wait(1)

	// теперь проверим что вы освободили порт и мы можем стартовать сервер ещё раз
	ctx, finish = context.WithCancel(context.Background())
	//fmt.Println("TestServerStartStop start 2 microservice:", time.Now().Format("15:04:05.000000000"))

	err = StartMyGrpcServer(ctx, listenAddr)
	if err != nil {
		t.Fatalf("cant start server again: %v", err)
	}
	wait(1)
	finish()
	wait(1)
}
