package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	pb "telegram-service/gen/telegram"
	"telegram-service/internal/colorlog"
	"telegram-service/internal/session"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrIncorrectSessID   = fmt.Errorf("invalid session_id")
	ErrIncorrectPhotoUrl = fmt.Errorf("invalid photo url")
	ErrIncorrectArgs     = fmt.Errorf("invalid arguments")
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

	log.Println(op, "success create for:", sessId)

	return &pb.CreateSessionResponse{SessionId: sessId, QrCode: qr}, nil
}

func (s *TelegramService) DeleteSession(ctx context.Context, req *pb.DeleteSessionRequest) (*pb.DeleteSessionResponse, error) {
	const op = "DeleteSession"

	sessId := req.SessionId
	if !session.IsValidSessionID(sessId) {
		return nil, status.Errorf(codes.InvalidArgument, ErrIncorrectArgs.Error())
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

	log.Println(op, "success for:", sessId)

	return &pb.DeleteSessionResponse{}, nil
}

func (s *TelegramService) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	const op = "SendMessage"

	sess, ok := s.mgr.Get(req.SessionId)
	if !ok {
		return nil, status.Error(codes.NotFound, "session not found")
	}

	log.Printf("%v: %+v\n%v %v\n", op, sess, req.Peer, req.Text)

	missing := make([]string, 0, 2)
	peer := req.Peer
	if peer == "" {
		missing = append(missing, "peer")
	}
	text := req.Text
	if text == "" {
		missing = append(missing, "text")
	}
	if len(missing) != 0 {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("%s (%s)", ErrIncorrectSessID, strings.Join(missing, ", ")))
	}

	messageID, err := sess.SendTo(peer, text)
	if err != nil {
		log.Println(op, "can't send message:", err)
		return nil, status.Errorf(codes.NotFound, "can't send message")
	}

	log.Println(op, "success for:", sess.GetID())

	return &pb.SendMessageResponse{MessageId: messageID}, nil
}

func (s *TelegramService) SendPhoto(ctx context.Context, req *pb.SendPhotoRequest) (*pb.SendPhotoResponse, error) {
	const op = "SendPhoto"

	sess, ok := s.mgr.Get(req.SessionId)
	if !ok {
		return nil, status.Error(codes.NotFound, "session not found")
	}

	log.Printf("%v: %+v\n%v %v\n", op, sess, req.Peer, req.Photo)

	missing := make([]string, 0, 2)
	peer := req.Peer
	if peer == "" {
		missing = append(missing, "peer")
	}
	photoUrl := req.Photo
	if photoUrl == "" {
		missing = append(missing, "photo url")
	}

	if len(missing) != 0 {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("%s (%s)", ErrIncorrectSessID, strings.Join(missing, ", ")))
	}

	ok = isURLValid(photoUrl)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, ErrIncorrectPhotoUrl.Error())
	}

	messageID, err := sess.SendPhoto(peer, photoUrl)
	if err != nil {
		log.Println(op, "can't send photo:", err)
		return nil, status.Errorf(codes.NotFound, "can't send photo")
	}

	log.Println(op, "success for:", sess.GetID())

	return &pb.SendPhotoResponse{MessageId: messageID}, nil
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

func isURLValid(str string) bool {
	u, err := url.ParseRequestURI(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}
