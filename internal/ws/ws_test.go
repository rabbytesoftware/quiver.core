package ws_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/ws"
)

type readResult struct {
	data []byte
	typ  ws.FrameType
	err  error
}

type fakeConn struct {
	mu sync.Mutex

	reads []readResult
	idx   int

	closeCalled bool
	closeCode   int
	closeReason string
}

func (f *fakeConn) Read(ctx context.Context) ([]byte, ws.FrameType, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Respect context cancellation
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	default:
	}

	if f.idx >= len(f.reads) {
		return nil, 0, io.EOF
	}

	r := f.reads[f.idx]
	f.idx++
	return r.data, r.typ, r.err
}

func (f *fakeConn) Write(context.Context, []byte, ws.FrameType) error {
	return nil
}

func (f *fakeConn) Close(code int, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closeCalled = true
	f.closeCode = code
	f.closeReason = reason
	return nil
}

type call struct {
	name string
	err  error
}

type mockHandler struct {
	calls []call

	onOpenErr    error
	onMessageErr error
}

func (m *mockHandler) OnOpen(ctx context.Context, c ws.Conn) error {
	m.calls = append(m.calls, call{name: "open"})
	return m.onOpenErr
}

func (m *mockHandler) OnMessage(ctx context.Context, c ws.Conn, _ []byte, _ ws.FrameType) error {
	m.calls = append(m.calls, call{name: "message"})
	return m.onMessageErr
}

func (m *mockHandler) OnError(ctx context.Context, c ws.Conn, err error) {
	m.calls = append(m.calls, call{name: "error", err: err})
}

func (m *mockHandler) OnClose(ctx context.Context, c ws.Conn, err error) {
	m.calls = append(m.calls, call{name: "close", err: err})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestServe(t *testing.T) {
	tests := []struct {
		name string

		ctx context.Context

		reads []readResult

		onOpenErr    error
		onMessageErr error

		wantCalls []string
		wantClose bool
	}{
		{
			name: "happy path, single message, EOF",
			ctx:  context.Background(),
			reads: []readResult{
				{[]byte("hi"), ws.TextFrame, nil},
				{nil, 0, io.EOF},
			},
			wantCalls: []string{
				"open",
				"message",
				"error",
				"close",
			},
		},
		{
			name:      "OnOpen error",
			ctx:       context.Background(),
			onOpenErr: errors.New("boom"),
			wantCalls: []string{
				"open",
				"close",
			},
			wantClose: true,
		},
		{
			name: "Read error",
			ctx:  context.Background(),
			reads: []readResult{
				{nil, 0, errors.New("read failed")},
			},
			wantCalls: []string{
				"open",
				"error",
				"close",
			},
		},
		{
			name: "OnMessage error",
			ctx:  context.Background(),
			reads: []readResult{
				{[]byte("msg"), ws.TextFrame, nil},
			},
			onMessageErr: errors.New("handler failed"),
			wantCalls: []string{
				"open",
				"message",
				"error",
				"close",
			},
			wantClose: true,
		},
		{
			name: "context canceled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			wantCalls: []string{
				"open",
				"error",
				"close",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &fakeConn{
				reads: tt.reads,
			}

			h := &mockHandler{
				onOpenErr:    tt.onOpenErr,
				onMessageErr: tt.onMessageErr,
			}

			ws.Serve(tt.ctx, conn, h)

			// Extract call names
			var got []string
			for _, c := range h.calls {
				got = append(got, c.name)
			}

			if !equalStrings(got, tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", got, tt.wantCalls)
			}

			if tt.wantClose && !conn.closeCalled {
				t.Fatalf("expected Conn.Close to be called")
			}
		})
	}
}
