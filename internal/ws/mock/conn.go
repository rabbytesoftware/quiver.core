package mock

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/ws"
)

type frame struct {
	typ  ws.FrameType
	data []byte
}

type frameHeader struct {
	Type   uint8
	Length uint32
}

type Conn struct {
	conn      net.Conn
	in        chan frame
	out       chan frame
	closed    chan struct{}
	peerDone  chan struct{}
	writeDone chan struct{}

	workers []ConnWorker

	once sync.Once
}

type ConnWorker func(ctx context.Context, c *Conn)

type ConnOption func(*Conn)

func WithWorker(w ConnWorker) ConnOption {
	return func(c *Conn) {
		c.workers = append(c.workers, w)
	}
}

func NewPair(ctx context.Context, opts ...ConnOption) (*Conn, *Conn) {
	a, b := net.Pipe()

	c1 := NewConn(ctx, a, opts...)
	c2 := NewConn(ctx, b, opts...)

	return c1, c2
}

func NewConn(ctx context.Context, nc net.Conn, opts ...ConnOption) *Conn {
	c := &Conn{
		conn:      nc,
		in:        make(chan frame, 16),
		out:       make(chan frame, 16),
		closed:    make(chan struct{}),
		peerDone:  make(chan struct{}),
		writeDone: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(c)
	}

	go c.readLoop(ctx)
	go c.writeLoop(ctx)

	for _, w := range c.workers {
		go w(ctx, c)
	}

	return c
}

func (c *Conn) Read(ctx context.Context) ([]byte, ws.FrameType, error) {
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()

	case <-c.closed:
		return nil, 0, errors.New("connection closed")

	case <-c.peerDone:
		return nil, 0, errors.New("peer closed connection")

	case f := <-c.in:
		return f.data, f.typ, nil
	}
}

func (c *Conn) Write(ctx context.Context, data []byte, typ ws.FrameType) error {
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

		// Wait for all queued writes to flush
		<-c.writeDone

		_ = c.conn.Close()
	})

	return nil
}

func (c *Conn) closePeer() {
	c.once.Do(func() {
		close(c.peerDone)
	})
}

func (c *Conn) writeLoop(ctx context.Context) {
	defer close(c.writeDone)

	for {
		select {
		case <-ctx.Done():
			return

		case <-c.closed:
			return

		case <-c.peerDone:
			return

		case f := <-c.out:
			if err := writeFrame(c.conn, f); err != nil {
				return
			}
		}
	}
}

var readFrameFn = readFrame

func (c *Conn) readLoop(ctx context.Context) {
	for {
		f, err := readFrameFn(c.conn)
		if err != nil {
			c.closePeer()
			return
		}

		select {
		case <-ctx.Done():
			c.closePeer()
			return

		case <-c.closed:
			c.closePeer()
			return

		case c.in <- f:
		}
	}
}

func writeFrame(w io.Writer, f frame) error {
	hdr := frameHeader{
		Type:   uint8(f.typ),
		Length: uint32(len(f.data)),
	}

	if err := binary.Write(w, binary.BigEndian, hdr); err != nil {
		return err
	}
	_, err := w.Write(f.data)
	return err
}

func readFrame(r io.Reader) (frame, error) {
	var hdr frameHeader

	if err := binary.Read(r, binary.BigEndian, &hdr); err != nil {
		return frame{}, err
	}

	data := make([]byte, hdr.Length)
	if _, err := io.ReadFull(r, data); err != nil {
		return frame{}, err
	}

	return frame{
		typ:  ws.FrameType(hdr.Type),
		data: data,
	}, nil
}

// HelloWorldWorker returns a ConnWorker that sends "hello world" messages
// at the specified interval. FOR TESTING PURPOSES ONLY.
func HelloWorldWorker(interval time.Duration) ConnWorker {
	return func(ctx context.Context, c *Conn) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-c.closed:
				return
			case <-c.peerDone:
				return
			case <-ticker.C:
				_ = c.Write(ctx, []byte("hello world"), ws.TextFrame)
			}
		}
	}
}
