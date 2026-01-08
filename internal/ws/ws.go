package ws

import "context"

type FrameType uint8

const (
	TextFrame FrameType = iota
	BinaryFrame
)

type Conn interface {
	Read(ctx context.Context) ([]byte, FrameType, error)
	Write(ctx context.Context, data []byte, typ FrameType) error
	Close(code int, reason string) error
}

type Dialer interface {
	Dial(ctx context.Context, addr string) (Conn, error)
}

type Upgrader interface {
	Upgrade(ctx context.Context) (Conn, error)
}
