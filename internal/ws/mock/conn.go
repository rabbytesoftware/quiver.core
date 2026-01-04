package mock

import (
	"context"
	"errors"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/ws"
)

type frame struct {
	data []byte
	typ  ws.MessageType
}

type Conn struct {
	in       chan frame
	out      chan frame
	closed   chan struct{}
	peerDone chan struct{}
	once     sync.Once
}

func newPair() (*Conn, *Conn) {
	aIn := make(chan frame, 16)
	bIn := make(chan frame, 16)

	aClosed := make(chan struct{})
	bClosed := make(chan struct{})

	return &Conn{
			in:       aIn,
			out:      bIn,
			closed:   aClosed,
			peerDone: bClosed,
		},
		&Conn{
			in:       bIn,
			out:      aIn,
			closed:   bClosed,
			peerDone: aClosed,
		}
}

func (c *Conn) Read(ctx context.Context) ([]byte, ws.MessageType, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	select {
	case <-c.closed:
		return nil, 0, errors.New("connection closed")
	default:
	}
	select {
	case <-c.peerDone:
		return nil, 0, errors.New("peer closed connection")
	default:
	}

	f := <-c.in
	return f.data, f.typ, nil

}

func (c *Conn) Write(ctx context.Context, data []byte, typ ws.MessageType) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-c.closed:
		return errors.New("connection closed")
	default:
	}
	select {
	case <-c.peerDone:
		return errors.New("peer closed connection")
	default:
	}

	c.out <- frame{data: data, typ: typ}
	return nil

}

func (c *Conn) Close(code int, reason string) error {
	c.once.Do(func() {
		close(c.closed)
	})
	return nil
}
