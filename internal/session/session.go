package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"telegram-service/internal/colorlog"
	"telegram-service/internal/config"
	"time"

	"github.com/google/uuid"
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

type Session struct {
	ID      string
	Client  *telegram.Client
	Updates chan *MessageUpdate
	mu      sync.RWMutex
	done    chan struct{}
}

type MessageUpdate struct {
	MessageID int64
	From      string
	Text      string
	Timestamp int64
}

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
	m.mu.RLock()
	client := telegram.NewClient(m.conf.AppId, m.conf.AppHash, telegram.Options{})
	m.mu.RUnlock()

	id := uuid.New().String()
	s := &Session{
		ID:      id,
		Client:  client,
		Updates: make(chan *MessageUpdate, 100),
		done:    make(chan struct{}),
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (s *Session) GetID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ID
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

func (m *Manager) Qr(sessId string) (string, error) {
	m.mu.RLock()
	s, ok := m.sessions[sessId]
	if !ok {
		return "", ErrNoSess
	}
	m.mu.RUnlock()

	dispatcher := tg.NewUpdateDispatcher()

	ctx := context.Background()

	// таймер 30 сек. на генерацию QR
	ctxWT, cancel := context.WithTimeout(ctx, time.Second*30)

	var qrChan = make(chan string, 1)
	defer close(qrChan)
	//var errChan = make(chan error, 1)
	//defer close(errChan)

	go func() {
		defer cancel()
		err := s.Client.Run(ctx, func(ctx context.Context) error {
			var isFirst = true
			_, err := s.Client.QR().Auth(ctx, qrlogin.OnLoginToken(dispatcher), func(ctx context.Context, token qrlogin.Token) error {
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
					//errChan <- ErrTimedOut
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
				log.Println("qr code:")
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
					return ErrQrTimedOut
				}

				if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
					return err
				}

				return err
			}

			user, err := s.Client.Self(ctx)
			if err != nil {
				return err
			}

			if status, err := s.Client.Auth().Status(ctx); status.Authorized {
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
					log.Println("can't delete temporary session:", delErr)
				}
				return err
			}

			return nil
		})

		if err != nil {
			log.Println("auth qr error:", err)
		}
	}()

	select {
	case <-ctxWT.Done():
		return "", ErrTimedOut
	case qrStr := <-qrChan:
		return qrStr, nil
	}
}

func (m *Manager) CheckSessionInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	log.Printf("\033[34mGetCheckSession:\033[0m\n")
	log.Printf("ctx: %+v \n", ctx)
	log.Printf("req: %+v \n", req)
	log.Printf("info: %+v \n", info)
	return handler(ctx, req)
}
