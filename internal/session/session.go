package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"telegram-service/internal/colorlog"
	"telegram-service/internal/e"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/skip2/go-qrcode"
)

var (
	ErrInvlidPeer = fmt.Errorf("invalid peer")
)

type Session struct {
	ID          string
	Client      *telegram.Client
	Updates     *messageUpdatePipe
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	requestPipe chan requestPipe
}

func NewSession(client *telegram.Client) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	id := uuid.New().String()
	reqPipe := make(chan requestPipe, 10)
	updates := newMessageUpdatePipe()
	return &Session{
		ID:          id,
		Client:      client,
		Updates:     updates,
		ctx:         ctx,
		cancel:      cancel,
		requestPipe: reqPipe,
	}
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

	s.requestPipe <- func() {
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
	}

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
				case f := <-s.requestPipe:
					go f()
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

	ctx, cancel := context.WithCancel(context.Background())
	ctxWT, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	var errChan = make(chan error, 1)
	var messageIdChan = make(chan int64, 1)
	defer close(messageIdChan)

	s.requestPipe <- func() {
		defer close(errChan)
		select {
		case <-ctxWT.Done():
			return
		default:
		}

		// id сообщения клиента
		id, randErr := crypto.RandInt64(crypto.DefaultRand())
		if err != nil {
			log.Println(e.Wrap(fmt.Sprintf("%s can't rand id:", op), randErr))
			errChan <- randErr
			return
		}

		// peer в int
		peer, cnvErr := strconv.Atoi(peerStr)
		if cnvErr != nil {
			log.Println(e.Wrap(fmt.Sprintf("%s can't convert to input peer:", op), cnvErr))
			errChan <- e.Wrap(ErrInvlidPeer.Error(), cnvErr)
			return
		}

		messageReq := &tg.MessagesSendMessageRequest{
			Peer:     &tg.InputPeerChat{ChatID: int64(peer)},
			Message:  text,
			RandomID: id,
		}
		colorlog.Solo("messageReq", messageReq)

		// Запрос
		_, sendErr := s.Client.API().MessagesSendMessage(s.ctx, messageReq)
		if sendErr != nil {
			log.Println(e.Wrap(op, cnvErr))
			errChan <- sendErr
		}
		messageIdChan <- id
	}

	select {
	case <-ctxWT.Done():
		return 0, ErrTimedOut
	case err = <-errChan:
		if err != nil {
			return 0, err
		}
	case messageID = <-messageIdChan:
	}

	return messageID, nil
}

func (s *Session) Run(f func(ctx context.Context) error) error {
	err := s.Client.Run(s.ctx, f)
	return err
}

func IsValidSessionID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
