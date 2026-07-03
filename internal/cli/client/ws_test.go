package client_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
)

func wsDaemon(t *testing.T, frames []string) (*httptest.Server, *record) {
	t.Helper()
	rec := &record{}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.path = r.URL.EscapedPath()
		conn, err := up.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		for _, f := range frames {
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(f)))
		}
		// Give the client a moment to drain before the deferred close.
		time.Sleep(50 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestSubscribeRuntime_ReceivesEvents(t *testing.T) {
	srv, rec := wsDaemon(t, []string{
		`{"namespace":"github.com/user/a","state":"installing"}`,
		`{"namespace":"github.com/user/a","state":"ready"}`,
	})
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := c.SubscribeRuntime(ctx, "github.com/user/a")
	require.NoError(t, err)

	first := <-events
	assert.Equal(t, "installing", first.State)
	second := <-events
	assert.Equal(t, "ready", second.State)

	assert.Equal(t, "/v0/runtime/github.com%2Fuser%2Fa", rec.path)
}

func TestSubscribeRuntime_ChannelClosesOnServerClose(t *testing.T) {
	srv, _ := wsDaemon(t, []string{`{"namespace":"github.com/user/a","state":"ready"}`})
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := c.SubscribeRuntime(ctx, "github.com/user/a")
	require.NoError(t, err)

	<-events // drain the one event
	select {
	case _, open := <-events:
		assert.False(t, open, "channel should close when the server closes")
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after server close")
	}
}

func TestSubscribeRuntime_CancelStopsStream(t *testing.T) {
	srv, _ := wsDaemon(t, nil)
	c := newClient(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	events, err := c.SubscribeRuntime(ctx, "github.com/user/a")
	require.NoError(t, err)

	cancel()
	select {
	case _, open := <-events:
		assert.False(t, open, "channel should close on context cancel")
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after cancel")
	}
}

func TestSubscribeRuntime_ConnErrorWhenDaemonDown(t *testing.T) {
	c, err := client.New("tcp://127.0.0.1:1")
	require.NoError(t, err)

	_, err = c.SubscribeRuntime(context.Background(), "github.com/user/a")
	require.Error(t, err)
	assert.Equal(t, 3, client.ExitCode(err))
}

func TestSubscribeRuntime_OverUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "quiver.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)

	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, uerr := up.Upgrade(w, r, nil)
		require.NoError(t, uerr)
		defer func() { _ = conn.Close() }()
		require.NoError(t, conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"namespace":"github.com/user/a","state":"ready"}`)))
		time.Sleep(50 * time.Millisecond)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	c, err := client.New("unix://" + sock)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := c.SubscribeRuntime(ctx, "github.com/user/a")
	require.NoError(t, err)

	evt := <-events
	assert.Equal(t, "ready", evt.State)
}

func TestSubscribeRuntime_SkipsMalformedFrames(t *testing.T) {
	srv, _ := wsDaemon(t, []string{
		`this is not json`,
		`{"namespace":"github.com/user/a","state":"ready"}`,
	})
	c := newClient(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := c.SubscribeRuntime(ctx, "github.com/user/a")
	require.NoError(t, err)

	evt := <-events
	assert.Equal(t, "ready", evt.State)
}
