package service

import (
	"context"
	"fmt"
	"log"
	pb "telegram-service/gen/telegram"
	"telegram-service/internal/colorlog"
	"telegram-service/internal/session"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrIncorrectSessID = fmt.Errorf("incorrect session_id")
)

type TelegramService struct {
	pb.UnimplementedTelegramServiceServer
	mgr *session.Manager
}

func NewTelegramService(mgr *session.Manager) *TelegramService {
	return &TelegramService{mgr: mgr}
}

func (s *TelegramService) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.CreateSessionResponse, error) {
	const op = "CreateSession"

	sess, err := s.mgr.Create(ctx)
	if err != nil {
		log.Println("error CreateSession:", err)
		return nil, status.Error(codes.Internal, "failed to create session")
	}
	colorlog.Solo("sess", sess)

	sessId := sess.GetID()
	qr, err := s.mgr.QR(sessId)
	if err != nil {
		log.Println("error CreateSession qr auth:", err)
		err := s.mgr.Delete(sessId)
		if err != nil {
			log.Println("error delete session:", err)
		}
		return nil, status.Error(codes.Internal, "failed to create qr for session")
	}

	storage, err := sess.SessionStorage.LoadSession(context.Background())
	colorlog.Multi("storage", storage, err)

	log.Println(op, "success create for:", sessId)

	return &pb.CreateSessionResponse{SessionId: sessId, QrCode: qr}, nil
}

func (s *TelegramService) DeleteSession(ctx context.Context, req *pb.DeleteSessionRequest) (*pb.DeleteSessionResponse, error) {
	const op = "DeleteSession"

	sessId := req.SessionId
	if !isValidSessionID(sessId) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid argument")
	}

	ok := s.mgr.Have(sessId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "session not found")
	}

	err := s.mgr.LogOut(sessId)
	if err != nil {
		log.Println(op, "can't log out:", err)
		return nil, status.Errorf(codes.NotFound, "session not found")
	}

	log.Println(op, "success log out for:", sessId)

	return &pb.DeleteSessionResponse{}, nil
}

func (s *TelegramService) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	const op = "SendMessage"

	sess, ok := s.mgr.Get(req.SessionId)
	if !ok {
		return nil, status.Error(codes.NotFound, "session not found")
	}

	log.Printf("%v: %+v\n", op, sess)
	// TODO: sess.Client.API().MessagesSendMessage(...)
	return &pb.SendMessageResponse{MessageId: 0}, status.Error(codes.Unimplemented, "TODO")
}

func (s *TelegramService) SubscribeMessages(req *pb.SubscribeMessagesRequest, srv pb.TelegramService_SubscribeMessagesServer) error {
	const op = "SubscribeMessages"

	sess, ok := s.mgr.Get(req.SessionId)
	if !ok {
		return status.Error(codes.NotFound, "session not found")
	}

	log.Printf("%v: %+v\n", op, sess)
	// TODO: stream from sess.Updates to srv.Send [web:4]
	return status.Error(codes.Unimplemented, "TODO: bidirectional stream")
}

func isValidSessionID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
