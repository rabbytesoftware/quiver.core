package surface_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

// serveOnSocket starts handler on ns's socket and returns the engine.
func serveOnSocket(t *testing.T, handler http.Handler) (surface.Surface, surface.Spec) {
	t.Helper()
	e := surface.New(shortDir(t))
	path, err := e.Prepare("github.com/user/chat")
	require.NoError(t, err)
	l, err := net.Listen("unix", path)
	require.NoError(t, err)
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close(); _ = os.Remove(path) })
	return e, surface.Spec{Mode: domainRuntime.SurfaceModeListen, Namespace: "github.com/user/chat"}
}

func TestProxy_ForwardsAndStripsCredentials(t *testing.T) {
	e, spec := serveOnSocket(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path + "|" + r.Header.Get("Authorization") + "|" + r.Header.Get("Cookie")))
	}))
	h, err := e.Handler(spec)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Cookie", "session=1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "/assets/app.js||", rec.Body.String())
}

func TestProxy_ArrowDownIs502(t *testing.T) {
	e := surface.New(shortDir(t))
	h, err := e.Handler(surface.Spec{Mode: domainRuntime.SurfaceModeListen, Namespace: "a/b"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestProxy_WebSocketUpgrade(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	e, spec := serveOnSocket(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.WriteMessage(mt, append([]byte("echo:"), msg...))
		}
	}))
	h, err := e.Handler(spec)
	require.NoError(t, err)
	front := httptest.NewServer(h)
	defer front.Close()

	url := "ws" + strings.TrimPrefix(front.URL, "http") + "/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn.Close()
	defer resp.Body.Close()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("hi")))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "echo:hi", string(msg))
}

func TestReady(t *testing.T) {
	e, spec := serveOnSocket(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // any HTTP answer means serving
	}))
	require.True(t, e.Ready(t.Context(), spec, "/"))

	down := surface.New(shortDir(t))
	require.False(t, down.Ready(t.Context(), surface.Spec{Mode: domainRuntime.SurfaceModeListen, Namespace: "x/y"}, "/"))
	require.True(t, down.Ready(t.Context(), surface.Spec{Mode: domainRuntime.SurfaceModeStatic}, "/"))
}

func TestReady_InvalidPathIsNotReady(t *testing.T) {
	e, spec := serveOnSocket(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	require.False(t, e.Ready(t.Context(), spec, "/a b\x7f"))
}
