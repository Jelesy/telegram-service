package service

import (
	"context"
	"errors"
	"log"
	pb "telegram-service/gen/telegram"
	"telegram-service/internal/session"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TelegramService struct {
	pb.UnimplementedTelegramServiceServer
	mgr *session.Manager
}

func NewTelegramService(mgr *session.Manager) *TelegramService {
	return &TelegramService{mgr: mgr}
}

func (s *TelegramService) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.CreateSessionResponse, error) {
	sess, err := s.mgr.Create(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create session")
	}
	// TODO: QR auth logic here
	qr := "TODO: generate QR with gotd/td QRLogin" // Используем telegram/auth/qrlogin.QR.Auth [web:2]
	return &pb.CreateSessionResponse{SessionId: sess.ID, QrCode: qr}, nil
}

func (s *TelegramService) DeleteSession(ctx context.Context, req *pb.DeleteSessionRequest) (*pb.DeleteSessionResponse, error) {
	const op = "DeleteSession"

	if req.SessionId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invalid argument")
	}

	err := s.mgr.Delete(req.SessionId)
	if err != nil {
		if errors.As(err, &session.ErrNoSess) {
			return nil, status.Errorf(codes.NotFound, "session not found")
		}
		log.Println(op, err)
		return nil, status.Errorf(codes.Unknown, "unknown error")
	}

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
