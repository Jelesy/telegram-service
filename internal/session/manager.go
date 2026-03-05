package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"telegram-service/internal/colorlog"
	"telegram-service/internal/config"
	"telegram-service/internal/e"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/skip2/go-qrcode"
	"google.golang.org/grpc"
)

var (
	ErrNoSess     = fmt.Errorf("no session")
	ErrQrTimedOut = fmt.Errorf("qr timed out")
	ErrTimedOut   = fmt.Errorf("timed out")
)

type Manager struct {
	sessions map[string]*Session
	conf     *config.Config
	mu       sync.RWMutex
}

func NewManager(conf *config.Config) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		conf:     conf,
		mu:       sync.RWMutex{},
	}
}

func (m *Manager) Create(ctx context.Context) (*Session, error) {
	storage := &session.StorageMemory{}
	id := uuid.New().String()
	s := &Session{
		ID: id,
		//Client:         client,
		SessionStorage: storage,
		Updates:        make(chan *MessageUpdate, 100),
		done:           make(chan struct{}),
	}

	m.Set(id, s)
	return s, nil
}

func (m *Manager) Have(id string) (ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok = m.sessions[id]
	return
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) Set(id string, s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[id] = s
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		close(s.done)
		delete(m.sessions, id)
		return nil
	} else {
		return ErrNoSess
	}
}

func (m *Manager) LogOut(sessId string) (err error) {
	const op = "LogOut"

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	client, err := m.NewTelegramClient(sessId)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = client.Run(ctx, func(ctx context.Context) error {
		_, err := client.API().AuthLogOut(ctx)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	// удаление локально
	err = m.Delete(sessId)
	if err != nil {
		log.Println(e.Wrap(op, err))
	}

	return nil
}

func (m *Manager) QR(sessId string) (qrStr string, err error) {
	const op = "QR"

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	dispatcher := tg.NewUpdateDispatcher()
	ctx := context.Background()

	// таймер 30 сек. на генерацию QR
	ctxWT, cancel := context.WithTimeout(ctx, time.Second*30)

	client, err := m.NewTelegramClient(sessId)
	if err != nil {
		cancel()
		return "", err
	}

	var qrChan = make(chan string, 1)
	defer close(qrChan)

	go func() {
		defer cancel()
		err := client.Run(ctx, func(ctx context.Context) error {
			var isFirst = true
			_, err := client.QR().Auth(ctx, qrlogin.OnLoginToken(dispatcher), func(ctx context.Context, token qrlogin.Token) error {
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

			if err != nil {
				delErr := m.Delete(sessId)
				if delErr != nil {
					log.Println("can't delete temporary session:", delErr)
				}
				if errors.Is(err, ErrQrTimedOut) {
					log.Println("gotd wrapped err:", err)
					m.Get(sessId)
					return ErrQrTimedOut
				}
				if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
					return err
				}
				return err
			}

			user, err := client.Self(ctx)
			if err != nil {
				return err
			}

			if status, err := client.Auth().Status(ctx); status.Authorized {
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
			} else {
				delErr := m.Delete(sessId)
				if delErr != nil {
					log.Println(op, "can't delete temporary session:", delErr)
				}
				return err
			}

			return nil
		})

		if err != nil {
			log.Println(op, "auth error:", err)
		} else {
			s, err := m.Get(sessId)
			st, stErr := s.SessionStorage.LoadSession(context.Background())
			colorlog.Multi(fmt.Sprint(op, "user"), s, err, st, stErr)
		}
	}()

	select {
	case <-ctxWT.Done():
		return "", ErrTimedOut
	case qrStr = <-qrChan:
		return qrStr, nil
	}
}

func (m *Manager) NewTelegramClient(sessID string) (*telegram.Client, error) {
	sess, ok := m.Get(sessID)
	if !ok {
		return nil, ErrNoSess
	}
	return telegram.NewClient(m.conf.AppId, m.conf.AppHash, telegram.Options{SessionStorage: sess.SessionStorage}), nil
}

func (m *Manager) CheckSessionInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	colorlog.Multi("request params", ctx, req, info)
	//log.Printf("\033[34mGetCheckSession:\033[0m\n")
	//log.Printf("ctx: %+v \n", ctx)
	//log.Printf("req: %+v \n", req)
	//log.Printf("info: %+v \n", info)
	return handler(ctx, req)
}
