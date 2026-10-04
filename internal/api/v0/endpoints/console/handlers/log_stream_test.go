package console

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func pairedConns(
	t *testing.T,
) (server, client *websocket.Conn) {
	t.Helper()

	accepted := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := middleware.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		accepted <- conn
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	server = <-accepted
	t.Cleanup(func() { _ = server.Close() })
	return server, client
}

func ringWith(
	records int,
) (logring.Ring, *slog.Logger) {
	ring := logring.New(1000)
	log := slog.New(ring.Tee(slog.NewTextHandler(discardWriter{}, nil)))
	for i := 0; i < records; i++ {
		log.Info("r", "i", i)
	}
	return ring, log
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func newStream(
	conn *websocket.Conn,
	ring logring.Ring,
) *logStream {
	return &logStream{
		conn:       conn,
		ring:       ring,
		params:     logParams{level: slog.LevelDebug, replay: defaultReplay},
		stuckLimit: time.Hour,
		pingEvery:  time.Hour,
	}
}

func readFrame(
	t *testing.T,
	conn *websocket.Conn,
) map[string]any {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var frame map[string]any
	require.NoError(t, conn.ReadJSON(&frame))
	return frame
}

func TestLogStream_Run_StopsWhenTheReplayCannotBeWritten(t *testing.T) {
	server, _ := pairedConns(t)
	ring, _ := ringWith(3)
	require.NoError(t, server.UnderlyingConn().Close())
	done := make(chan struct{})

	go func() {
		newStream(server, ring).run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after a failed replay write")
	}
}

func TestLogStream_Run_StopsWhenTheReadyFrameCannotBeWritten(t *testing.T) {
	server, _ := pairedConns(t)
	ring, _ := ringWith(0)
	require.NoError(t, server.UnderlyingConn().Close())

	newStream(server, ring).run(context.Background())
}

func TestLogStream_SendLive_PrecedesARecordWithTheGapItMissed(t *testing.T) {
	server, client := pairedConns(t)
	ring, log := ringWith(0)
	sub := ring.Subscribe(slog.LevelDebug)
	defer sub.Close()
	for i := 0; i < 300; i++ {
		log.Info("flood")
	}
	rec := <-sub.C()

	require.NoError(t, newStream(server, ring).sendLive(sub, rec))

	gap := readFrame(t, client)
	assert.Equal(t, "gap", gap["type"])
	assert.Equal(t, float64(300-256), gap["dropped"])
	assert.Equal(t, "log", readFrame(t, client)["type"])
}

func TestLogStream_SendLive_ReportsAWriteFailureOnTheGap(t *testing.T) {
	server, _ := pairedConns(t)
	ring, log := ringWith(0)
	sub := ring.Subscribe(slog.LevelDebug)
	defer sub.Close()
	for i := 0; i < 300; i++ {
		log.Info("flood")
	}
	rec := <-sub.C()
	require.NoError(t, server.UnderlyingConn().Close())

	assert.Error(t, newStream(server, ring).sendLive(sub, rec))
}

func TestLogStream_SendRecord_SkipsWhatWasAlreadySent(t *testing.T) {
	server, client := pairedConns(t)
	ring, _ := ringWith(2)
	stream := newStream(server, ring)
	records := ring.Snapshot(0, slog.LevelDebug, 10)

	require.NoError(t, stream.sendRecord(records[1]))
	require.NoError(t, stream.sendRecord(records[0]))

	assert.Equal(t, float64(2), readFrame(t, client)["seq"])
	assert.Equal(t, uint64(2), stream.last)
}

func TestLogStream_Step_StopsOnEachTerminalSignal(t *testing.T) {
	ring, _ := ringWith(0)
	pings := make(chan time.Time)
	never := make(chan struct{})

	t.Run("a closed subscription", func(t *testing.T) {
		server, _ := pairedConns(t)
		sub := ring.Subscribe(slog.LevelDebug)
		sub.Close()

		assert.False(t, newStream(server, ring).step(context.Background(), sub, pings, never))
	})

	t.Run("a finished reader", func(t *testing.T) {
		server, _ := pairedConns(t)
		sub := ring.Subscribe(slog.LevelDebug)
		defer sub.Close()
		done := make(chan struct{})
		close(done)

		assert.False(t, newStream(server, ring).step(context.Background(), sub, pings, done))
	})

	t.Run("a cancelled context", func(t *testing.T) {
		server, _ := pairedConns(t)
		sub := ring.Subscribe(slog.LevelDebug)
		defer sub.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		assert.False(t, newStream(server, ring).step(ctx, sub, pings, never))
	})

	t.Run("a ping that cannot be written", func(t *testing.T) {
		server, _ := pairedConns(t)
		sub := ring.Subscribe(slog.LevelDebug)
		defer sub.Close()
		require.NoError(t, server.UnderlyingConn().Close())
		ticks := make(chan time.Time, 1)
		ticks <- time.Now()

		assert.False(t, newStream(server, ring).step(context.Background(), sub, ticks, never))
	})
}

func TestLogStream_Step_ContinuesAfterAHealthyPingAndARecord(t *testing.T) {
	server, client := pairedConns(t)
	ring, log := ringWith(0)
	sub := ring.Subscribe(slog.LevelDebug)
	defer sub.Close()
	stream := newStream(server, ring)
	pinged := make(chan struct{}, 1)
	client.SetPingHandler(func(string) error {
		pinged <- struct{}{}
		return nil
	})
	go func() {
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()
	never := make(chan struct{})

	assert.True(t, stream.step(context.Background(), sub, ticks, never))
	<-pinged
	log.Info("live")
	assert.True(t, stream.step(context.Background(), sub, ticks, never))
}

func TestLogStream_StartReader_AcceptsPongsAndEndsWhenTheClientCloses(t *testing.T) {
	server, client := pairedConns(t)
	ring, _ := ringWith(0)
	done := newStream(server, ring).startReader()

	require.NoError(t, client.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second)))
	require.NoError(t, client.Close())

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the reader did not end when the client closed")
	}
}
