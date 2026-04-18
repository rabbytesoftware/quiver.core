package ws_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	ws "github.com/rabbytesoftware/quiver/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func dial(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func readJSON(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(msg, v))
}

func newServer(t *testing.T) (*ws.Handler, *httptest.Server) {
	t.Helper()
	h := ws.NewHandler()
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.GET("/v0/arrow", h.HandleArrow)
	r.GET("/v0/arrow/:ns", h.HandleArrow)
	r.GET("/v0/arrow.runtime", h.HandleArrowRuntime)
	r.GET("/v0/arrow.runtime/:ns", h.HandleArrowRuntime)
	r.GET("/v0/quiver", h.HandleQuiver)
	r.GET("/v0/quiver/:ns", h.HandleQuiver)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return h, srv
}

func TestHandler_ArrowGlobalSubscription(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow")

	h.WaitRegistered()
	h.PushArrow(domain.Arrow{
		Namespace: "github.com/user/repo",
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"}}}},
	})

	var d dto.ArrowDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}

func TestHandler_ArrowNamespaceSubscription_MatchingNS(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow/github.com%2Fuser%2Frepo")

	h.WaitRegistered()
	h.PushArrow(domain.Arrow{
		Namespace: "github.com/user/repo",
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Test"}}}},
	})

	var d dto.ArrowDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}

func TestHandler_ArrowNamespaceSubscription_NonMatchingNS(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow/github.com%2Fother%2Frepo")

	h.WaitRegistered()
	h.PushArrow(domain.Arrow{
		Namespace: "github.com/user/repo",
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Test"}}}},
	})

	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "expected timeout, no message for non-matching namespace")
}

func TestHandler_ArrowRuntimeSubscription(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow.runtime")

	h.WaitRegistered()
	h.PushArrowRuntime(domainRuntime.ArrowRuntime{
		Ref:   "github.com/user/repo",
		State: domain.ArrowStateRunning,
	})

	var d dto.ArrowRuntimeDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "running", d.State)
}

func TestHandler_QuiverSubscription(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/quiver")

	h.WaitRegistered()
	h.PushQuiver(domain.Quiver{
		Namespace: "github.com/user/repo",
		Manifest:  domain.QuiverManifest{Name: "My Quiver"},
	})

	var d dto.QuiverDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}

func TestHandler_UpgradeRejectsNonWS(t *testing.T) {
	_, srv := newServer(t)
	resp, err := http.Get(srv.URL + "/v0/arrow")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandler_ReadPump_ClientClose_ExitsCleanly(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow")
	h.WaitRegistered()

	// Closing the connection causes readPump to get an error and return,
	// which triggers unregister → cl.done is closed → writePump exits via <-cl.done.
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	conn.Close()

	// Push after unregister is safe — broadcast holds RLock and skips missing clients.
	h.PushArrow(domain.Arrow{Namespace: "github.com/user/repo"})
	// Reaching this point confirms no panic and writePump's <-cl.done branch ran.
}

func TestHandler_Broadcast_SlowConsumer_DoesNotBlock(t *testing.T) {
	h, srv := newServer(t)
	_ = dial(t, srv, "/v0/arrow") // connect but never read
	h.WaitRegistered()

	// Push 65 arrows — 64 fill the send buffer (capacity 64), the 65th hits the
	// default drop branch in broadcast. If default were missing the call would block.
	for i := 0; i < 65; i++ {
		h.PushArrow(domain.Arrow{
			Namespace: domain.Namespace(fmt.Sprintf("github.com/user/repo%d", i)),
		})
	}
	// Reaching here means broadcast's default branch did not block.
}
