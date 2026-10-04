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

func seeded(n int) (*logring.Ring, *slog.Logger) {
	ring := logring.New()
	log := slog.New(ring.Wrap(slog.NewTextHandler(&strings.Builder{}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	for range n {
		log.Info("old", "component", "release", "k", "v")
	}
	return ring, log
}

func connect(t *testing.T, ring *logring.Ring, query string) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	ended := make(chan struct{}, 1)
	h := New(ring, "v")
	r := gin.New()
	r.GET("/logs", func(c *gin.Context) {
		h.Logs(c)
		ended <- struct{}{}
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/logs"+query, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	return conn, ended
}

func next(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	var frame map[string]any
	require.NoError(t, conn.ReadJSON(&frame))
	return frame
}

func TestHandlers_Logs_ReplaysThenSendsReadyThenFollowsLive(t *testing.T) {
	ring, log := seeded(2)
	conn, _ := connect(t, ring, "")

	first, _, ready := next(t, conn), next(t, conn), next(t, conn)
	log.Warn("fresh")
	live := next(t, conn)

	assert.Equal(t, map[string]any{"type": "log", "seq": float64(1), "time": first["time"], "level": "info", "component": "release", "msg": "old", "fields": map[string]any{"k": "v"}}, first)
	assert.Equal(t, map[string]any{"type": "ready", "seq": float64(2)}, ready)
	assert.Equal(t, "fresh", live["msg"])
	assert.Equal(t, float64(3), live["seq"])
}

func TestHandlers_Logs_HonoursSinceLevelAndFlagsAReset(t *testing.T) {
	testCases := []struct {
		query string
		want  []map[string]any
	}{
		{"?since=1", []map[string]any{{"type": "log", "seq": float64(2)}, {"type": "ready", "seq": float64(2)}}},
		{"?level=error", []map[string]any{{"type": "ready", "seq": float64(2)}}},
		{"?since=50", []map[string]any{{"seq": float64(1)}, {"seq": float64(2)}, {"type": "ready", "seq": float64(2), "reset": true}}},
	}
	for _, tc := range testCases {
		t.Run(tc.query, func(t *testing.T) {
			ring, _ := seeded(2)
			conn, _ := connect(t, ring, tc.query)

			for _, want := range tc.want {
				got := next(t, conn)
				for k, v := range want {
					assert.Equal(t, v, got[k], k)
				}
			}
		})
	}
}

func TestHandlers_Logs_RejectsBadParameters(t *testing.T) {
	for _, query := range []string{"?level=loud", "?since=-1"} {
		r := gin.New()
		r.GET("/logs", New(logring.New(), "v").Logs)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs"+query, nil))

		assert.Equal(t, http.StatusBadRequest, rec.Code, query)
	}
}

func TestHandlers_Logs_EndsWhenTheClientHangsUp(t *testing.T) {
	ring, _ := seeded(0)
	conn, ended := connect(t, ring, "")
	require.Equal(t, "ready", next(t, conn)["type"])

	require.NoError(t, conn.Close())

	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream did not end")
	}
}
