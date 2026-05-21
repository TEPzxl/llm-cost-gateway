package email

import (
	"context"
	"sync"
)

type FakeSender struct {
	mu       sync.Mutex
	messages []Message
	err      error
}

func NewFakeSender() *FakeSender {
	return &FakeSender{}
}

func (s *FakeSender) Send(_ context.Context, message Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, message)
	return nil
}

func (s *FakeSender) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make([]Message, len(s.messages))
	copy(messages, s.messages)
	return messages
}

func (s *FakeSender) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}
