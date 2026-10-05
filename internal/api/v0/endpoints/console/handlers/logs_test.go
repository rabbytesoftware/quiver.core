package console

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func seededRing(
	count int,
) (logring.Ring, *slog.Logger) {
	ring := logring.New()
	log := slog.New(ring.Wrap(slog.NewTextHandler(&strings.Builder{}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	for i := 0; i < count; i++ {
		log.Info("old", "component", "release", "k", "v")
	}
	return ring, log
}

func connectLogs(
	t *testing.T,
	ring logring.Ring,
	query string,
) *websocket.Conn {
	t.Helper()

	r := gin.New()
	r.GET("/logs", New(ring, "v").Logs)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/logs" + query
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() {
		_ = conn.Close()
	})
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	return conn
}

func nextFrame(
	t *testing.T,
	conn *websocket.Conn,
) map[string]any {
	t.Helper()

	var frame map[string]any
	require.NoError(t, conn.ReadJSON(&frame))
	return frame
}

func assertFramesStartWith(
	t *testing.T,
	query string,
	want []map[string]any,
) {
	t.Helper()

	ring, _ := seededRing(2)
	conn := connectLogs(t, ring, query)

	for _, expected := range want {
		assertFrameHas(t, nextFrame(t, conn), expected, query)
	}
}

func assertFrameHas(
	t *testing.T,
	got map[string]any,
	expected map[string]any,
	label string,
) {
	t.Helper()

	for key, value := range expected {
		assert.Equal(t, value, got[key], label+" "+key)
	}
}

func TestHandlers_Logs_Connected_ReplaysThenSendsReadyThenFollowsLive(
	t *testing.T,
) {
	ring, log := seededRing(2)
	conn := connectLogs(t, ring, "")

	first := nextFrame(t, conn)
	nextFrame(t, conn)
	ready := nextFrame(t, conn)
	log.Warn("fresh")
	live := nextFrame(t, conn)

	assert.Equal(t, map[string]any{"type": "log", "seq": float64(1), "time": first["time"], "level": "info", "component": "release", "msg": "old", "fields": map[string]any{"k": "v"}}, first)
	assert.Equal(t, map[string]any{"type": "ready", "seq": float64(2)}, ready)
	assert.Equal(t, "fresh", live["msg"])
	assert.Equal(t, float64(3), live["seq"])
}

func TestHandlers_Logs_SinceAndLevel_FilterTheReplay(
	t *testing.T,
) {
	assertFramesStartWith(t, "?since=1", []map[string]any{{"type": "log", "seq": float64(2)}, {"type": "ready", "seq": float64(2)}})
	assertFramesStartWith(t, "?level=error", []map[string]any{{"type": "ready", "seq": float64(2)}})
	assertFramesStartWith(t, "?level=WARN", []map[string]any{{"type": "ready", "seq": float64(2)}})
}

func TestHandlers_Logs_SinceNewerThanTheDaemon_ReplaysAllAndFlagsReset(
	t *testing.T,
) {
	assertFramesStartWith(t, "?since=50", []map[string]any{
		{"seq": float64(1)}, {"seq": float64(2)}, {"type": "ready", "seq": float64(2), "reset": true},
	})
}

func assertBadQuery(
	t *testing.T,
	query string,
) {
	t.Helper()

	r := gin.New()
	r.GET("/logs", New(logring.New(), "v").Logs)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs"+query, nil))

	assert.Equal(t, http.StatusBadRequest, rec.Code, query)
}

func TestHandlers_Logs_BadParameters_AreRefused(
	t *testing.T,
) {
	assertBadQuery(t, "?level=loud")
	assertBadQuery(t, "?since=-1")
	assertBadQuery(t, "?since=x")
}

func TestHandlers_Logs_NotAWebSocket_IsIgnored(
	t *testing.T,
) {
	r := gin.New()
	r.GET("/logs", New(logring.New(), "v").Logs)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs", nil))

	assert.NotEqual(t, http.StatusSwitchingProtocols, rec.Code)
}

func TestHandlers_Logs_ClientHangsUp_ReleasesTheSubscription(
	t *testing.T,
) {
	stream := newFakeStream()
	conn := connectLogs(t, &fakeRing{stream: stream}, "")

	require.NoError(t, conn.Close())

	select {
	case <-stream.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the subscription was not released")
	}
}

func TestHandlers_Logs_SubscriptionEndsOnItsOwn_DisconnectsTheClient(
	t *testing.T,
) {
	stream := newFakeStream()
	conn := connectLogs(t, &fakeRing{stream: stream}, "")
	require.Equal(t, "ready", nextFrame(t, conn)["type"])
	close(stream.live)

	_, _, err := conn.ReadMessage()

	assert.Error(t, err)
	select {
	case <-stream.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the subscription was not released")
	}
}
