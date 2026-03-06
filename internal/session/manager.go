package session

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"telegram-service/internal/colorlog"
	"telegram-service/internal/config"
	"telegram-service/internal/e"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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

func (m *Manager) Create(ctx context.Context) (s *Session, err error) {
	const op = "Create"

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	client, err := m.NewDefaultTelegramClient()
	if err != nil {
		return nil, err
	}

	s = NewSession(client)

	err = s.StartClientSession()
	if err != nil {
		return nil, err
	}

	m.Set(s)

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

func (m *Manager) Set(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessID := s.GetID()
	m.sessions[sessID] = s
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; ok {
		delete(m.sessions, id)
		return nil
	} else {
		return ErrNoSess
	}
}

func (m *Manager) LogOut(sessID string) (err error) {
	const op = "LogOut"

	defer func() {
		err = e.WrapIfErr(op, err)
	}()

	s, ok := m.Get(sessID)
	if !ok {
		return ErrNoSess
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctxWT, cancel := context.WithTimeout(ctx, time.Second*30)

	defer cancel()
	var errChan = make(chan error, 1)

	s.requestPipe <- func() {
		defer close(errChan)
		select {
		case <-ctxWT.Done():
			return
		default:
		}
		_, err := s.Client.API().AuthLogOut(s.ctx)
		errChan <- err
	}

	select {
	case err = <-errChan:
		if err != nil {
			return err
		}
	case <-ctxWT.Done():
		return ErrTimedOut
	}

	// удаление локально
	err = m.Delete(sessID)
	if err != nil {
		log.Println(e.Wrap(op, err))
	}

	return nil
}

func (m *Manager) NewDefaultTelegramClient(opts ...telegram.Options) (*telegram.Client, error) {
	stor := &session.StorageMemory{}
	go func() {
		timeStart := time.Now().Format(time.TimeOnly)
		timer := time.NewTimer(time.Second * 20)
		defer timer.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				storData, err := stor.LoadSession(context.Background())
				strStorData := strings.ReplaceAll(string(storData), "\\", "")
				colorlog.Multi("storage check", timeStart, strStorData, err)
			}
		}

	}()
	deviceConf := telegram.DeviceConfig{
		DeviceModel:   "my app server",
		SystemVersion: "server v0.1",
	}
	return telegram.NewClient(m.conf.AppId, m.conf.AppHash, telegram.Options{
		SessionStorage: stor,
		Device:         deviceConf,
	}), nil
}

func (m *Manager) UnaryCheckSessionInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	md, _ := metadata.FromIncomingContext(ctx)
	colorlog.Multi("unary request params", md, req, info)
	return handler(ctx, req)
}

func (m *Manager) StreamCheckSessionInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	md, _ := metadata.FromIncomingContext(ss.Context())
	colorlog.Multi("stream request params", srv, md, ss, info)
	return handler(srv, ss)
}
