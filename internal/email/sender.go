package email

import "context"

type Message struct {
	To       string
	Subject  string
	TextBody string
}

type Sender interface {
	Send(ctx context.Context, message Message) error
}
