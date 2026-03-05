package session

import (
	"sync"

	"github.com/gotd/td/session"
)

type Session struct {
	ID string
	//Client         *telegram.Client
	Updates        chan *MessageUpdate
	SessionStorage *session.StorageMemory
	mu             sync.RWMutex
	done           chan struct{}
}

type MessageUpdate struct {
	MessageID int64
	From      string
	Text      string
	Timestamp int64
}

func (s *Session) GetID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ID
}
