package mock

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/ws"
)

type Listener struct {
	conns chan ws.Conn
}

func NewListener() *Listener {
	return &Listener{conns: make(chan ws.Conn, 8)}
}

func (l *Listener) Upgrade(ctx context.Context) (ws.Conn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-l.conns:
		return conn, nil
	}
}

type Dialer struct {
	listener *Listener
}

func NewDialer(l *Listener) *Dialer {
	return &Dialer{listener: l}
}

func (d *Dialer) Dial(ctx context.Context, addr string) (ws.Conn, error) {
	c1, c2 := newPair()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case d.listener.conns <- c2:
		return c1, nil
	}
}
