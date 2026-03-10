package session

type messageUpdatePipe struct {
	pipe chan *messageUpdate
}

type messageUpdate struct {
	MessageID int64
	From      string
	Text      string
	Timestamp int64
}

func newMessageUpdatePipe() *messageUpdatePipe {
	return &messageUpdatePipe{pipe: make(chan *messageUpdate, 50)}
}
