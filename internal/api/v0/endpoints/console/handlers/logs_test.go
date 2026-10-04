package console_test

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	handlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

type logsFixture struct {
	ring   logring.Ring
	log    *slog.Logger
	server *httptest.Server
}

type smallBufferListener struct{ net.Listener }

func (l smallBufferListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetWriteBuffer(2048)
	}
	return conn, err
}

func newLogsFixture(
	t *testing.T,
	opts ...handlers.Option,
) *logsFixture {
	t.Helper()

	ring := logring.New(100)
	r := gin.New()
	r.GET("/console/logs", handlers.New(ring, command.New(command.Options{}), opts...).Logs)
	server := httptest.NewUnstartedServer(r)
	server.Listener = smallBufferListener{server.Listener}
	server.Start()
	t.Cleanup(server.Close)

	log := slog.New(ring.Tee(slog.NewTextHandler(discard{}, nil)))
	return &logsFixture{ring: ring, log: log, server: server}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func (f *logsFixture) dial(
	t *testing.T,
	query string,
) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/console/logs" + query
	conn, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func next(
	t *testing.T,
	conn *websocket.Conn,
) map[string]any {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var frame map[string]any
	require.NoError(t, conn.ReadJSON(&frame))
	return frame
}

func TestHandlers_Logs_InvalidParametersAre400(t *testing.T) {
	f := newLogsFixture(t)

	for _, query := range []string{"?level=trace", "?level=INFO", "?since=-1", "?since=abc", "?replay=-1", "?replay=x"} {
		resp, err := http.Get(f.server.URL + "/console/logs" + query)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, query)
	}
}

func TestHandlers_Logs_ReplaysThenSendsReadyThenGoesLive(t *testing.T) {
	f := newLogsFixture(t)
	f.log.Info("one", "component", "daemon", "k", "v")
	f.log.Warn("two")

	conn := f.dial(t, "")

	first := next(t, conn)
	assert.Equal(t, "log", first["type"])
	assert.Equal(t, float64(1), first["seq"])
	assert.Equal(t, "info", first["level"])
	assert.Equal(t, "daemon", first["component"])
	assert.Equal(t, "one", first["msg"])
	assert.Equal(t, map[string]any{"k": "v"}, first["fields"])
	assert.NotEmpty(t, first["time"])
	assert.Equal(t, false, first["fields_truncated"])
	assert.Equal(t, "two", next(t, conn)["msg"])
	ready := next(t, conn)
	assert.Equal(t, map[string]any{"type": "ready", "seq": float64(2)}, ready)

	f.log.Error("live")
	live := next(t, conn)
	assert.Equal(t, "log", live["type"])
	assert.Equal(t, "live", live["msg"])
	assert.Equal(t, float64(3), live["seq"])
}

func TestHandlers_Logs_ReadyComesFirstWhenThereIsNothingToReplay(t *testing.T) {
	f := newLogsFixture(t)

	conn := f.dial(t, "")

	assert.Equal(t, map[string]any{"type": "ready", "seq": float64(0)}, next(t, conn))
}

func TestHandlers_Logs_LevelFiltersReplayAndLiveRecords(t *testing.T) {
	f := newLogsFixture(t)
	f.log.Debug("d")
	f.log.Info("i")
	f.log.Warn("w")

	conn := f.dial(t, "?level=warn")

	assert.Equal(t, "w", next(t, conn)["msg"])
	assert.Equal(t, "ready", next(t, conn)["type"])
	f.log.Info("skipped")
	f.log.Error("e")
	assert.Equal(t, "e", next(t, conn)["msg"])
}

func TestHandlers_Logs_SinceReplaysOnlyNewerRecords(t *testing.T) {
	f := newLogsFixture(t)
	for _, msg := range []string{"a", "b", "c"} {
		f.log.Info(msg)
	}

	conn := f.dial(t, "?since=2")

	assert.Equal(t, "c", next(t, conn)["msg"])
	assert.Equal(t, float64(3), next(t, conn)["seq"])
}

func TestHandlers_Logs_ReplayLimitsHowManyRecordsComeBack(t *testing.T) {
	f := newLogsFixture(t)
	for _, msg := range []string{"a", "b", "c", "d"} {
		f.log.Info(msg)
	}

	conn := f.dial(t, "?replay=2")

	assert.Equal(t, "c", next(t, conn)["msg"])
	assert.Equal(t, "d", next(t, conn)["msg"])
	assert.Equal(t, "ready", next(t, conn)["type"])
}

func TestHandlers_Logs_ReplayZeroSkipsStraightToReady(t *testing.T) {
	f := newLogsFixture(t)
	f.log.Info("old")

	conn := f.dial(t, "?replay=0")

	assert.Equal(t, "ready", next(t, conn)["type"])
}

func TestHandlers_Logs_ASinceFromBeforeADaemonRestartIsDiscardedAndFlagged(t *testing.T) {
	f := newLogsFixture(t)
	f.log.Info("fresh")

	conn := f.dial(t, "?since=5000")

	assert.Equal(t, "fresh", next(t, conn)["msg"])
	assert.Equal(t, map[string]any{"type": "ready", "seq": float64(1), "reset": true}, next(t, conn))
}

func TestHandlers_Logs_RecordsLoggedDuringReplayAreNeitherLostNorRepeated(t *testing.T) {
	f := newLogsFixture(t)
	for i := 0; i < 50; i++ {
		f.log.Info("m", "i", i)
	}
	conn := f.dial(t, "")
	go func() {
		for i := 50; i < 100; i++ {
			f.log.Info("m", "i", i)
		}
	}()

	var seqs []float64
	for len(seqs) < 100 {
		frame := next(t, conn)
		if frame["type"] == "log" {
			seqs = append(seqs, frame["seq"].(float64))
		}
	}

	for i, seq := range seqs {
		assert.Equal(t, float64(i+1), seq)
	}
}

func TestHandlers_Logs_RedactedValuesNeverReachTheWire(t *testing.T) {
	f := newLogsFixture(t)
	f.log.Info("auth", "token", "abc", "url", "https://u:pw@h/x")

	conn := f.dial(t, "")

	frame := next(t, conn)
	raw, err := json.Marshal(frame)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "abc")
	assert.NotContains(t, string(raw), "pw@")
	assert.Contains(t, string(raw), logring.Redacted)
}

func TestHandlers_Logs_AClosedClientReleasesItsSubscription(t *testing.T) {
	f := newLogsFixture(t)
	conn := f.dial(t, "")
	next(t, conn)

	require.NoError(t, conn.Close())

	assert.Eventually(t, func() bool {
		f.log.Info("poke")
		sub := f.ring.Subscribe(slog.LevelDebug)
		defer sub.Close()
		return true
	}, time.Second, 10*time.Millisecond)
}

func flood(
	log *slog.Logger,
	records int,
) {
	pad := strings.Repeat("x", 2000)
	for i := 0; i < records; i++ {
		log.Info("flood", "pad", pad)
	}
}

func TestHandlers_Logs_AClientThatStopsReadingMissesRecordsAndIsToldSo(t *testing.T) {
	f := newLogsFixture(t, handlers.WithStuckLimit(time.Hour))
	conn := f.dial(t, "")
	next(t, conn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		flood(f.log, 3000)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("logging blocked on a client that stopped reading")
	}
	assert.Equal(t, uint64(3000), f.ring.Latest())

	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		_, data, err := conn.ReadMessage()
		require.NoError(t, err, "the stream must stay open and report a gap")
		if strings.Contains(string(data), `"type":"gap"`) {
			return
		}
	}
}

func TestHandlers_Logs_AClientStalledForTooLongIsDisconnectedWithoutBlockingTheDaemon(t *testing.T) {
	f := newLogsFixture(t, handlers.WithStuckLimit(100*time.Millisecond))
	conn := f.dial(t, "")
	next(t, conn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 40; i++ {
			flood(f.log, 100)
			time.Sleep(20 * time.Millisecond)
		}
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("logging blocked on a client that stopped reading")
	}

	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.False(t, os.IsTimeout(err), "the server must close the stalled connection, not leave it hanging")
			return
		}
	}
}
