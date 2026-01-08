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

type Handler interface {
	OnOpen(ctx context.Context, c Conn) error

	OnMessage(ctx context.Context, c Conn, data []byte, typ FrameType) error

	OnError(ctx context.Context, c Conn, err error)

	OnClose(ctx context.Context, c Conn, err error)
}

type HandlerFactory interface {
	New(ctx context.Context, c Conn) Handler
}

func Serve(ctx context.Context, c Conn, h Handler) {
	if err := h.OnOpen(ctx, c); err != nil {
		_ = c.Close(1002, err.Error())
		h.OnClose(ctx, c, err)
		return
	}

	for {
		data, typ, err := c.Read(ctx)
		if err != nil {
			h.OnError(ctx, c, err)
			h.OnClose(ctx, c, err)
			return
		}

		if err := h.OnMessage(ctx, c, data, typ); err != nil {
			h.OnError(ctx, c, err)
			_ = c.Close(1002, err.Error())
			h.OnClose(ctx, c, err)
			return
		}
	}
}
