package mock

import (
	"context"
	"errors"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/ws"
)

type state struct {
	closed chan struct{}
	once   sync.Once
}

type frame struct {
	data []byte
	typ  ws.MessageType
}

type Conn struct {
	in  chan frame
	out chan frame
	st  *state
}

func newPair() (*Conn, *Conn) {
	aIn := make(chan frame, 16)

	bIn := make(chan frame, 16)

	st := &state{
		closed: make(chan struct{}),
	}

	return &Conn{
			in:  aIn,
			out: bIn,
			st:  st,
		},
		&Conn{
			in:  bIn,
			out: aIn,
			st:  st,
		}
}

func (c *Conn) Read(ctx context.Context) ([]byte, ws.MessageType, error) {
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case <-c.st.closed:
		return nil, 0, errors.New("connection closed")
	case f := <-c.in:
		return f.data, f.typ, nil
	}
}

func (c *Conn) Write(ctx context.Context, data []byte, typ ws.MessageType) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.st.closed:
		return errors.New("connection closed")
	case c.out <- frame{data: data, typ: typ}:
		return nil
	}
}

func (c *Conn) Close(code int, reason string) error {
	c.st.once.Do(func() {
		close(c.st.closed)
	})
	return nil
}
