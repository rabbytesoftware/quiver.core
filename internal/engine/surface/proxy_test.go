package surface_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

// serveOnSocket starts handler on ns's socket and returns the engine.
func serveOnSocket(
	t *testing.T,
	handler http.Handler,
) (surface.Surface, surface.Spec) {
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

func TestProxy_ForwardsOnlyTheSanitisedQuery(t *testing.T) {
	e, spec := serveOnSocket(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RawQuery))
	}))
	h, err := e.Handler(spec)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/q?a=1;b=2&c=3", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "c=3", rec.Body.String())
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

// stableGoroutines returns the goroutine count once it stops changing.
func stableGoroutines() int {
	n, same := runtime.NumGoroutine(), 0
	for same < 5 {
		time.Sleep(20 * time.Millisecond)
		if m := runtime.NumGoroutine(); m == n {
			same++
		} else {
			n, same = m, 0
		}
	}
	return n
}

// settledGoroutines waits for the goroutine count to stop exceeding limit.
func settledGoroutines(limit int) int {
	deadline := time.Now().Add(3 * time.Second)
	n := runtime.NumGoroutine()
	for n > limit && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

func TestReady_RepeatedProbesDoNotLeakGoroutines(t *testing.T) {
	e, spec := serveOnSocket(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	require.True(t, e.Ready(t.Context(), spec, "/"))
	base := stableGoroutines()

	for range 50 {
		require.True(t, e.Ready(t.Context(), spec, "/"))
	}
	require.LessOrEqual(t, settledGoroutines(base+5), base+5)
}

func TestHandler_CachesProxyPerNamespace(t *testing.T) {
	e := surface.New(shortDir(t))
	spec := surface.Spec{Mode: domainRuntime.SurfaceModeListen, Namespace: "a/b"}

	first, err := e.Handler(spec)
	require.NoError(t, err)
	second, err := e.Handler(spec)
	require.NoError(t, err)
	other, err := e.Handler(surface.Spec{Mode: domainRuntime.SurfaceModeListen, Namespace: "c/d"})
	require.NoError(t, err)

	require.Same(t, first, second)
	require.NotSame(t, first, other)
}

func TestCleanup_EvictsProxyAndClosesIdleConnections(t *testing.T) {
	e, spec := serveOnSocket(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	h, err := e.Handler(spec)
	require.NoError(t, err)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	idle := runtime.NumGoroutine()

	e.Cleanup(spec.Namespace)

	require.LessOrEqual(t, settledGoroutines(idle-1), idle-1, "the pooled connection must be closed")
	fresh, err := e.Handler(spec)
	require.NoError(t, err)
	require.NotSame(t, h, fresh)
}

func TestProxy_RepeatedRequestsDoNotGrowGoroutines(t *testing.T) {
	e, spec := serveOnSocket(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	h, err := e.Handler(spec)
	require.NoError(t, err)
	serve := func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusOK, rec.Code)
	}
	serve()
	base := stableGoroutines()

	for range 100 {
		serve()
	}
	require.LessOrEqual(t, settledGoroutines(base+5), base+5)
}
