package mock_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/ws"
	"github.com/rabbytesoftware/quiver/internal/ws/mock"
)

func TestMockWebSocketLifecycle(t *testing.T) {
	listener := mock.NewListener()
	dialer := mock.NewDialer(listener)

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)

	var srvErr error
	go func() {
		defer wg.Done()

		srvConn, err := listener.Upgrade(ctx)
		if err != nil {
			srvErr = err
			return
		}
		defer srvConn.Close(1000, "server closed")

		msg, typ, err := srvConn.Read(ctx)
		if err != nil {
			srvErr = err
			return
		}

		srvErr = srvConn.Write(ctx, msg, typ)
	}()

	clientConn, err := dialer.Dial(ctx, "mock://ws")
	if err != nil {
		t.Fatal("Dial failed:", err)
	}
	defer clientConn.Close(1000, "client closed")

	if err := clientConn.Write(ctx, []byte("hello"), ws.Text); err != nil {
		t.Fatal("Write failed:", err)
	}

	msg, typ, err := clientConn.Read(ctx)
	if err != nil {
		t.Fatal("Read failed:", err)
	}
	if string(msg) != "hello" || typ != ws.Text {
		t.Errorf("Expected message 'hello' and type Text, got message '%s' and type %d", string(msg), typ)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if srvErr != nil {
			t.Fatal(srvErr)
		}
	case <-time.After(time.Second):
		t.Fatal("server goroutine timed out")
	}
}
