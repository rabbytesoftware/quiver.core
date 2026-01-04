package mock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/ws"
)

func TestMockWebSocketLifecycle(t *testing.T) {
	listener := NewListener()
	dialer := NewDialer(listener)

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

func TestMockWebSocketContextCancellation(t *testing.T) {
	listener := NewListener()
	dialer := NewDialer(listener)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := dialer.Dial(ctx, "mock://ws")
	if err == nil {
		t.Fatal("Expected Dial to fail due to context cancellation")
	}

	_, err = listener.Upgrade(ctx)
	if err == nil {
		t.Fatal("Expected Upgrade to fail due to context cancellation")
	}
}

func TestNewPairCreatesConnectedConns(t *testing.T) {
	c1, c2 := newPair()
	defer c1.Close(1000, "test done")
	defer c2.Close(1000, "test done")

	ctx := context.Background()
	testMsg := []byte("test message")

	if err := c1.Write(ctx, testMsg, ws.Text); err != nil {
		t.Fatal("c1 Write failed:", err)
	}
	msg, typ, err := c2.Read(ctx)
	if err != nil {
		t.Fatal("c2 Read failed:", err)
	}
	if string(msg) != string(testMsg) || typ != ws.Text {
		t.Errorf("Expected message '%s' and type Text, got message '%s' and type %d", string(testMsg), string(msg), typ)
	}

	if err := c2.Write(ctx, testMsg, ws.Binary); err != nil {
		t.Fatal("c2 Write failed:", err)
	}
	msg, typ, err = c1.Read(ctx)
	if err != nil {
		t.Fatal("c1 Read failed:", err)
	}
	if string(msg) != string(testMsg) || typ != ws.Binary {
		t.Errorf("Expected message '%s' and type Binary, got message '%s' and type %d", string(testMsg), string(msg), typ)
	}
}

func TestReadConnectionClosed(t *testing.T) {
	c1, c2 := newPair()

	ctx := context.Background()
	c1.Close(1000, "closing connection")

	_, _, err := c2.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to closed connection")
	}
}
func TestWriteConnectionClosed(t *testing.T) {
	c1, c2 := newPair()

	ctx := context.Background()
	c1.Close(1000, "closing connection")
	time.Sleep(10 * time.Millisecond) // Ensure close propagates

	err := c2.Write(ctx, []byte("data"), ws.Text)
	if err == nil {
		t.Fatal("Expected Write to fail due to closed peer:")
	}

	_, _, err = c2.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to closed peer")
	}

	c2.Close(1000, "closing peer")
	time.Sleep(10 * time.Millisecond) // Ensure close propagates

	err = c1.Write(ctx, []byte("data"), ws.Text)
	if err == nil {
		t.Fatal("Expected Write to fail due to own closed connection:")
	}

	_, _, err = c1.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to own closed connection")
	}
}

func TestCloseIdempotent(t *testing.T) {
	c1, c2 := newPair()

	err1 := c1.Close(1000, "first close")
	err2 := c2.Close(1000, "second close")

	if err1 != nil {
		t.Fatal("First Closed failed:", err1)
	}
	if err2 != nil {
		t.Fatal("Second Close failed:", err2)
	}
}

func TestContextCancellation(t *testing.T) {
	listener := NewListener()
	dialer := NewDialer(listener)
	c1, c2 := newPair()
	defer c1.Close(1000, "test done")
	defer c2.Close(1000, "test done")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()                         // Cancel immediately
	time.Sleep(5 * time.Millisecond) // Ensure cancellation propagates

	_, _, err := c1.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to context cancellation")
	}

	err = c2.Write(ctx, []byte("data"), ws.Text)
	if err == nil {
		t.Fatal("Expected Write to fail due to context cancellation")
	}

	_, err = dialer.Dial(ctx, "mock://ws")
	if err == nil {
		t.Fatal("Expected Dial to fail due to context cancellation")
	}

	_, err = listener.Upgrade(ctx)
	if err == nil {
		t.Fatal("Expected Upgrade to fail due to context cancellation")
	}
}

func TestClose(t *testing.T) {
	c1, c2 := newPair()

	err := c1.Close(1000, "closing connection")
	if err != nil {
		t.Fatal("Close failed:", err)
	}

	err = c2.Close(1000, "closing peer")
	if err != nil {
		t.Fatal("Close failed:", err)
	}

	err = c1.Close(1000, "closing again")
	if err != nil {
		t.Fatal("Second Close failed:", err)
	}
}
