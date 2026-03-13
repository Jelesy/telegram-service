package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	pb "telegram-service/gen/telegram"
	"telegram-service/internal/colorlog"
	"telegram-service/internal/e"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/skip2/go-qrcode"
)

var (
	ErrInvlidPeer = fmt.Errorf("invalid peer")
	ErrSend       = fmt.Errorf("can't send message")
	ErrAlreadySub = fmt.Errorf("already subscribed")
)

type Session struct {
	ID string

	// Messages
	subscribed bool
	msgChan    chan *pb.MessageUpdate

	Client *telegram.Client

	mu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

func NewSession(appId int, appHash string) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	id := uuid.New().String()
	msgChannel := make(chan *pb.MessageUpdate, 50)

	sess := &Session{
		ID:      id,
		ctx:     ctx,
		cancel:  cancel,
		msgChan: msgChannel,
	}

	client := sess.NewDefaultTelegramClient(appId, appHash)
	sess.Client = client

	return sess
}

func (s *Session) Stop() {
	s.cancel()
	close(s.msgChan)
}

func (s *Session) GetSub() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subscribed
}

func (s *Session) SetSub(val bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribed = val
}

func (s *Session) GetID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ID
}

func (m *Manager) QR(sessID string) (qrStr string, err error) {
	const op = "QR"

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	s, ok := m.Get(sessID)
	if !ok {
		return "", ErrNoSess
	}

	dispatcher := tg.NewUpdateDispatcher()
	ctx := context.Background()

	// таймер 30 сек. на генерацию QR
	ctxWT, cancel := context.WithTimeout(ctx, time.Second*30)

	var qrChan = make(chan string, 1)
	defer close(qrChan)
	var errChan = make(chan error, 1)

	go func() {
		defer cancel()
		var isFirst = true
		_, err := s.Client.QR().Auth(s.ctx, qrlogin.OnLoginToken(dispatcher), func(ctx context.Context, token qrlogin.Token) error {
			// Первый QR никто не сканировал - новый не нужен
			if !isFirst {
				return ErrQrTimedOut
			}
			isFirst = false

			// Проверка на долгое ожидание qr
			// (Передается не этот контекст, потому что иначе прервется авторизация на нашей стороне,
			// а пользователь может отсканировать qr)
			select {
			case <-ctxWT.Done():
				return ErrTimedOut
			default:
			}

			colorlog.Solo("токен истекает", token.Expires().Format(time.Layout))

			qr, err := qrcode.New(token.URL(), qrcode.Medium)
			if err != nil {
				return err
			}

			colorlog.Solo("qr url", token.URL())
			qrChan <- token.URL()

			qrCode := qr.ToSmallString(false)
			fmt.Println("qr code:")
			fmt.Print(qrCode)

			return nil
		})
		defer close(errChan)
		if err != nil {
			errChan <- err
			delErr := m.Delete(sessID)
			if delErr != nil {
				log.Println(e.Wrap(fmt.Sprint(op, "can't delete temporary session:"), delErr))
			}
			if errors.Is(err, ErrQrTimedOut) {
				log.Println(e.Wrap(op, err))
				return
			}
			if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
				log.Println(e.Wrap(op, err))
				return
			}
			return
		}

		user, err := s.Client.Self(ctx)
		if err != nil {
			log.Println(e.Wrap(op, err))
		}

		if status, err := s.Client.Auth().Status(s.ctx); status.Authorized {
			fmt.Printf(
				"Login successfully!\n"+
					"ID: %v,\n"+
					"Username: %s,\n"+
					"First name: %s,\n"+
					"Last name: %s,\n"+
					"Status: %s,\n"+
					"Premium: %v,\n",
				user.ID,
				user.Username,
				user.FirstName,
				user.LastName,
				user.Status,
				user.Premium,
			)
			colorlog.Solo("user", user)
			//st, er := s.Client.Config().SetAutologinToken.
			colorlog.Multi("storage")
		} else {
			delErr := m.Delete(sessID)
			if delErr != nil {
				log.Println(op, "can't delete temporary session:", delErr)
			}
			log.Println(e.Wrap(op, err))
			return
		}
	}()

	select {
	case err = <-errChan:
		return "", err
	case <-ctxWT.Done():
		return "", ErrTimedOut
	case qrStr = <-qrChan:
		return qrStr, nil
	}
}

func (s *Session) StartClientSession() error {
	const op = "StartClientSession"

	waitCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	started := make(chan error)
	defer close(started)

	go func() {
		err := s.Run(func(sessCtx context.Context) error {
			select {
			case <-waitCtx.Done():
				return ErrTimedOut
			default:
			}

			started <- nil
			for {
				select {
				case <-sessCtx.Done():
					return nil
				}
			}
		})

		if err != nil {
			select {
			case <-waitCtx.Done():
			default:
				started <- err
			}
			log.Println(e.WrapIfErr(fmt.Sprint(op, "stop client session"), err))
		}

		started <- err
		log.Println(e.WrapIfErr(fmt.Sprint(op, "stop client session"), err))
	}()

	select {
	case <-waitCtx.Done():
		return e.Wrap(op, ErrTimedOut)
	case ansErr := <-started:
		if ansErr != nil {
			return e.Wrap(op, ansErr)
		}
	}

	return nil
}

func (s *Session) SendTo(peerStr, text string) (messageID int64, err error) {
	const op = "SendTo"

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	sender := message.NewSender(s.Client.API())
	req := sender.Resolve(peerStr)
	updates, sendErr := req.Text(s.ctx, text)
	if sendErr != nil {
		log.Println(e.Wrap(op, sendErr))
		return 0, err
	}

	messageID = getMessageId(updates)

	return messageID, nil
}

func (s *Session) SendPhoto(peerStr, photoUrl string) (messageID int64, err error) {
	const op = "SendPhoto"

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	sender := message.NewSender(s.Client.API())
	req := sender.Resolve(peerStr)
	updates, sendErr := req.PhotoExternal(s.ctx, photoUrl)
	if sendErr != nil {
		log.Println(e.Wrap(op, sendErr))
		return 0, sendErr
	}

	messageID = getMessageId(updates)

	return messageID, nil
}

func (s *Session) Subscribe(srv pb.TelegramService_SubscribeMessagesServer) (err error) {
	const op = "Subscribe"
	log.Println(op, s.ID)

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	if s.GetSub() {
		return ErrAlreadySub
	}

	log.Println(op, s.ID, "subscribed")
	s.SetSub(true)
	defer func() {
		log.Println(op, s.ID, "unsubscribed")
		s.SetSub(false)
	}()

	for {
		select {
		case <-s.ctx.Done():
			log.Println(op, "sess ctx", e.ErrCtxDone.Error())
			return e.ErrCtxDone
		case <-srv.Context().Done():
			log.Println(op, "req ctx", e.ErrCtxDone.Error())
			return e.ErrCtxDone
		case msg := <-s.msgChan:
			sendErr := srv.Send(msg)
			if sendErr != nil {
				return ErrSend
			}
		}
	}
}

func (s *Session) NewDefaultTelegramClient(appId int, appHash string, opts ...telegram.Options) *telegram.Client {
	store := &session.StorageMemory{}
	dispatcher := s.NewMsgUpdatesDispatcher()
	deviceConf := telegram.DeviceConfig{
		DeviceModel:   "my app server",
		SystemVersion: "server v0.1",
	}
	return telegram.NewClient(appId, appHash, telegram.Options{
		SessionStorage: store,
		Device:         deviceConf,
		UpdateHandler:  dispatcher,
	})
}

func (s *Session) NewMsgUpdatesDispatcher() tg.UpdateDispatcher {
	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
		const op = "Session.UpdateDispatcher"
		log.Println(op, s.ID)

		select {
		case <-s.ctx.Done():
			log.Println(e.ErrCtxDone.Error())
			return e.ErrCtxDone
		default:
			if !s.GetSub() {
				return nil
			}
			// продолжаем
		}

		msg, ok := update.Message.(*tg.Message)
		if !ok {
			log.Println(op, "update.Message convert *tg.Message not ok")
			return nil
		}

		if msg.Out {
			return nil
		}

		msg.GetFromID()
		peer, ok := msg.FromID.(*tg.PeerUser)
		if !ok {
			log.Println(op, "msg.FromID convert *tg.PeerUser not ok")
			return nil
		}

		userFrom, ok := entities.Users[peer.UserID]
		if !ok {
			return nil
		}

		colorlog.Multi(fmt.Sprint(op, "new message"), s.ID, msg, peer, userFrom)

		msgUpdate := &pb.MessageUpdate{
			MessageId: int64(msg.ID),
			From:      fmt.Sprintf("%s %s [%s]", userFrom.FirstName, userFrom.LastName, userFrom.Username),
			Text:      msg.Message,
			Timestamp: int64(msg.Date),
		}

		s.msgChan <- msgUpdate

		return nil
	})
	return dispatcher
}

func (s *Session) Run(f func(ctx context.Context) error) error {
	err := s.Client.Run(s.ctx, f)
	return err
}

func IsValidSessionID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func getMessageId(updates tg.UpdatesClass) int64 {
	if updates, ok := updates.(*tg.Updates); ok {
		upds := updates.GetUpdates()
		for _, upd := range upds {
			if updMessID, ok := upd.(*tg.UpdateMessageID); ok {
				return int64(updMessID.ID)
			}
		}
	}
	return 0
}
