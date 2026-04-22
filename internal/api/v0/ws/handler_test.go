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

func TestNewHandler_CreatesValidHandler(t *testing.T) {
	h := ws.NewHandler()
	require.NotNil(t, h)
	assert.NotNil(t, h.Arrow)
	assert.NotNil(t, h.Runtime)
	assert.NotNil(t, h.Quiver)
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
	r.GET("/v0/arrow", h.Arrow.Handle)
	r.GET("/v0/arrow/:ns", h.Arrow.Handle)
	r.GET("/v0/arrow.runtime", h.Runtime.Handle)
	r.GET("/v0/arrow.runtime/:ns", h.Runtime.Handle)
	r.GET("/v0/quiver", h.Quiver.Handle)
	r.GET("/v0/quiver/:ns", h.Quiver.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return h, srv
}

// TestHandler_Arrow_UserInstalled_DefaultFilter verifies that connecting without
// ?user_installed receives only user-installed arrows.
func TestHandler_Arrow_UserInstalled_DefaultFilter(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow")

	h.Arrow.WaitRegistered()
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/user/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Test", Version: "1.0.0"},
		UserInstalled: true,
	})

	var d dto.ArrowDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}

// TestHandler_Arrow_UserInstalled_True filters to user-installed only.
func TestHandler_Arrow_UserInstalled_True(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow?user_installed=true")

	h.Arrow.WaitRegistered()

	// dep arrow — should not be delivered
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/user/dep",
		ArrowMeta:     domain.ArrowMeta{Name: "Dep"},
		UserInstalled: false,
	})
	// user-installed arrow — should be delivered
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/user/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Test"},
		UserInstalled: true,
	})

	var d dto.ArrowDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}

// TestHandler_Arrow_UserInstalled_False filters to deps only.
func TestHandler_Arrow_UserInstalled_False(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow?user_installed=false")

	h.Arrow.WaitRegistered()

	// user-installed arrow — should not be delivered
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/user/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Test"},
		UserInstalled: true,
	})
	// dep arrow — should be delivered
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/user/dep",
		ArrowMeta:     domain.ArrowMeta{Name: "Dep"},
		UserInstalled: false,
	})

	var d dto.ArrowDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/dep", d.Namespace)
}

// TestHandler_Arrow_DefaultFilter_DepDropped verifies that deps are silently dropped
// when no filter param is given (default = user-installed only).
func TestHandler_Arrow_DefaultFilter_DepDropped(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow")

	h.Arrow.WaitRegistered()
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/user/dep",
		UserInstalled: false,
	})

	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "expected timeout — dep arrow must not be delivered by default")
}

func TestHandler_ArrowRuntimeSubscription(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow.runtime")

	h.Runtime.WaitRegistered()
	h.PushArrowRuntime(domainRuntime.ArrowRuntime{
		Ref:   "github.com/user/repo",
		State: domain.ArrowStateRunning,
	})

	var d dto.ArrowRuntimeDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "running", d.State)
}

func TestHandler_ArrowRuntime_NamespaceFilter_Matching(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow.runtime/github.com%2Fuser%2Frepo")

	h.Runtime.WaitRegistered()
	h.PushArrowRuntime(domainRuntime.ArrowRuntime{
		Ref:   "github.com/user/repo",
		State: domain.ArrowStateRunning,
	})

	var d dto.ArrowRuntimeDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}

func TestHandler_ArrowRuntime_NamespaceFilter_NonMatching(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow.runtime/github.com%2Fother%2Frepo")

	h.Runtime.WaitRegistered()
	h.PushArrowRuntime(domainRuntime.ArrowRuntime{
		Ref:   "github.com/user/repo",
		State: domain.ArrowStateRunning,
	})

	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "expected timeout — non-matching namespace must not be delivered")
}

func TestHandler_QuiverSubscription(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/quiver")

	h.Quiver.WaitRegistered()
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
	h.Arrow.WaitRegistered()

	// Closing the connection causes readPump to get an error and return,
	// which triggers unregister → cl.done is closed → writePump exits via <-cl.done.
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	conn.Close()

	// Push after unregister is safe — broadcaster holds RLock and skips missing clients.
	h.PushArrow(domain.Arrow{Namespace: "github.com/user/repo", UserInstalled: true})
	// Reaching this point confirms no panic and writePump's <-cl.done branch ran.
}

func TestHandler_Broadcast_SlowConsumer_DoesNotBlock(t *testing.T) {
	h, srv := newServer(t)
	_ = dial(t, srv, "/v0/arrow?user_installed=true") // connect but never read
	h.Arrow.WaitRegistered()

	// Push 65 user-installed arrows — 64 fill the send buffer (capacity 64), the 65th hits
	// the default drop branch in PushArrow. If default were missing the call would block.
	for i := 0; i < 65; i++ {
		h.PushArrow(domain.Arrow{
			Namespace:     domain.Namespace(fmt.Sprintf("github.com/user/repo%d", i)),
			UserInstalled: true,
		})
	}
	// Reaching here means broadcast's default branch did not block.
}

func TestHandler_Arrow_NamespaceGlob(t *testing.T) {
	h, srv := newServer(t)
	conn := dial(t, srv, "/v0/arrow/github.com%2Fuser%2Frepo")

	h.Arrow.WaitRegistered()
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/other/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Other"},
		UserInstalled: true,
	})
	h.PushArrow(domain.Arrow{
		Namespace:     "github.com/user/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Target"},
		UserInstalled: true,
	})

	var d dto.ArrowDTO
	readJSON(t, conn, &d)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}
