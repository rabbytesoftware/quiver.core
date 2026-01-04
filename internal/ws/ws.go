package ws

import "context"

type MessageType int

const (
	Text MessageType = iota
	Binary
)

type Conn interface {
	Read(ctx context.Context) ([]byte, MessageType, error)
	Write(ctx context.Context, data []byte, typ MessageType) error
	Close(code int, reason string) error
}

type Dialer interface {
	Dial(ctx context.Context, addr string) (Conn, error)
}

type Upgrader interface {
	Upgrade(ctx context.Context) (Conn, error)
}
