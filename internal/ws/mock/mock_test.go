package mock

import (
	"bytes"
	"context"
	"io"
	"net"
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

	if err := clientConn.Write(ctx, []byte("hello"), ws.TextFrame); err != nil {
		t.Fatal("Write failed:", err)
	}

	msg, typ, err := clientConn.Read(ctx)
	if err != nil {
		t.Fatal("Read failed:", err)
	}
	if string(msg) != "hello" || typ != ws.TextFrame {
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
	ctx := context.Background()
	c1, c2 := NewPair(ctx)
	defer c1.Close(1000, "test done")
	defer c2.Close(1000, "test done")

	testMsg := []byte("test message")

	if err := c1.Write(ctx, testMsg, ws.TextFrame); err != nil {
		t.Fatal("c1 Write failed:", err)
	}
	msg, typ, err := c2.Read(ctx)
	if err != nil {
		t.Fatal("c2 Read failed:", err)
	}
	if string(msg) != string(testMsg) || typ != ws.TextFrame {
		t.Errorf("Expected message '%s' and type Text, got message '%s' and type %d", string(testMsg), string(msg), typ)
	}

	if err := c2.Write(ctx, testMsg, ws.TextFrame); err != nil {
		t.Fatal("c2 Write failed:", err)
	}
	msg, typ, err = c1.Read(ctx)
	if err != nil {
		t.Fatal("c1 Read failed:", err)
	}
	if string(msg) != string(testMsg) || typ != ws.TextFrame {
		t.Errorf("Expected message '%s' and type Binary, got message '%s' and type %d", string(testMsg), string(msg), typ)
	}
}

func TestReadConnectionClosed(t *testing.T) {
	ctx := context.Background()
	c1, c2 := NewPair(ctx)
	c1.Close(1000, "closing connection")

	_, _, err := c2.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to closed connection")
	}
}
func TestWriteConnectionClosed(t *testing.T) {
	ctx := context.Background()
	c1, c2 := NewPair(ctx)

	c1.Close(1000, "closing connection")
	time.Sleep(10 * time.Millisecond) // Ensure close propagates

	err := c2.Write(ctx, []byte("data"), ws.TextFrame)
	if err == nil {
		t.Fatal("Expected Write to fail due to closed peer:")
	}

	_, _, err = c2.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to closed peer")
	}

	c2.Close(1000, "closing peer")
	time.Sleep(10 * time.Millisecond) // Ensure close propagates

	err = c1.Write(ctx, []byte("data"), ws.TextFrame)
	if err == nil {
		t.Fatal("Expected Write to fail due to own closed connection:")
	}

	_, _, err = c1.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to own closed connection")
	}
}

func TestCloseIdempotent(t *testing.T) {
	ctx := context.Background()
	c1, c2 := NewPair(ctx)

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
	c1, c2 := NewPair(context.Background())
	defer c1.Close(1000, "test done")
	defer c2.Close(1000, "test done")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()                         // Cancel immediately
	time.Sleep(5 * time.Millisecond) // Ensure cancellation propagates

	_, _, err := c1.Read(ctx)
	if err == nil {
		t.Fatal("Expected Read to fail due to context cancellation")
	}

	err = c2.Write(ctx, []byte("data"), ws.TextFrame)
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
	c1, c2 := NewPair(context.Background())

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

func TestWriteReadFrame(t *testing.T) {
	var buf bytes.Buffer

	want := frame{typ: 0, data: []byte("hi")}

	if err := writeFrame(&buf, want); err != nil {
		t.Fatal(err)
	}

	got, err := readFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got.data, want.data) {
		t.Fatalf("got %q, want %q", got.data, want.data)
	}
}

func TestHelloWorldWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, client := net.Pipe()
	defer client.Close()

	_ = NewConn(
		ctx,
		server,
		WithWorker(HelloWorldWorker(10*time.Millisecond)),
	)

	// read raw frame from the client side
	f, err := readFrame(client)
	if err != nil {
		t.Fatal(err)
	}

	if string(f.data) != "hello world" {
		t.Fatalf("got %q, want hello world", f.data)
	}
}

func TestReadLoop_ContextDone(t *testing.T) {
	c := &Conn{
		in:       make(chan frame),
		closed:   make(chan struct{}),
		peerDone: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	readFrameFn = func(_ io.Reader) (frame, error) {
		return frame{data: []byte("hello")}, nil
	}

	defer func() { readFrameFn = readFrame }()

	go c.readLoop(ctx)

	time.Sleep(10 * time.Millisecond) // Allow goroutine to exit

	cancel()

	time.Sleep(10 * time.Millisecond) // Allow cancellation to propagate
	select {
	case <-c.peerDone:
	// success
	case <-time.After(time.Second):
		t.Fatal("peerDone was not closed on ctx.Done")
	}

	select {
	case <-c.in:
		t.Fatal("frame should not be delivered after ctx.Done")
	default:
		// success
	}
}

func TestReadLoop_ConnClosed(t *testing.T) {
	c := &Conn{
		in:       make(chan frame),
		closed:   make(chan struct{}),
		peerDone: make(chan struct{}),
	}
	ctx := context.Background()

	readFrameFn = func(_ io.Reader) (frame, error) {
		return frame{data: []byte("hello")}, nil
	}
	defer func() { readFrameFn = readFrame }()

	go c.readLoop(ctx)

	// Simulate local Close()
	close(c.closed)

	select {
	case <-c.peerDone:
		// OK
	case <-time.After(time.Second):
		t.Fatal("peerDone was not closed on c.closed")
	}

	select {
	case <-c.in:
		t.Fatal("frame should not be delivered after c.closed")
	default:
		// OK
	}
}
