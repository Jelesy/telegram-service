package session

import (
	"context"
	"fmt"
	"log"
	"sync"
	"telegram-service/internal/config"

	"github.com/google/uuid"
	"github.com/gotd/td/telegram"
	_ "github.com/gotd/td/telegram/auth"
	"google.golang.org/grpc"
)

var (
	ErrNoSess = fmt.Errorf("no session")
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
	id := uuid.New().String()
	s := &Session{
		ID:      id,
		Updates: make(chan *MessageUpdate, 100),
		done:    make(chan struct{}),
	}
	// TODO: init client with QR auth
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

func (m *Manager) CheckSessionInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	log.Printf("\033[34mGetCheckSession:\033[0m\n")
	log.Printf("ctx: %+v \n", ctx)
	log.Printf("req: %+v \n", req)
	log.Printf("info: %+v \n", info)
	return handler(ctx, req)
}
