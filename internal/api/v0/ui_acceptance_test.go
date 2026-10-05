package v0_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

const uiNamespace = "github.com/quiver/chat"

// listenRuntime answers GetRuntime with an execution that has a listen
// surface open, for every namespace.
type listenRuntime struct {
	usecases.RuntimeUsecase
}

func (listenRuntime) GetRuntime(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
	return &domainRuntime.ArrowRuntime{
		Ref: ns,
		Execution: &domainRuntime.Execution{ID: "e1", Surface: &domainRuntime.Surface{
			Mode: domainRuntime.SurfaceModeListen, Path: "/",
		}},
	}, nil
}

// idleRuntime knows every arrow and has nothing running for any of them.
type idleRuntime struct {
	usecases.RuntimeUsecase
}

func (idleRuntime) GetRuntime(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
	return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, nil
}

// arrowSocket is a stand-in arrow: an HTTP server on the unix socket the
// surface engine proxies to.
type arrowSocket struct {
	mu          sync.Mutex
	gotAuth     string
	gotCookie   string
	gotPath     string
	gotRawQuery string
}

func (a *arrowSocket) seen() (auth, cookie, path, query string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gotAuth, a.gotCookie, a.gotPath, a.gotRawQuery
}

func startArrowSocket(t *testing.T, socket string) *arrowSocket {
	t.Helper()

	arrow := &arrowSocket{}
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			typ, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if conn.WriteMessage(typ, msg) != nil {
				return
			}
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		arrow.mu.Lock()
		arrow.gotAuth = r.Header.Get("Authorization")
		arrow.gotCookie = r.Header.Get("Cookie")
		arrow.gotPath = r.URL.Path
		arrow.gotRawQuery = r.URL.RawQuery
		arrow.mu.Unlock()
		_, _ = io.WriteString(w, "hello from the arrow")
	})

	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)
	srv := &httptest.Server{Listener: ln, Config: &http.Server{Handler: mux, ReadHeaderTimeout: 5e9}}
	srv.Start()
	t.Cleanup(srv.Close)
	return arrow
}

// newUIEnv is the daemon bound as if over tcp:// (bearer required) with the
// surface usecase backed by a real engine and a real socket server.
func newUIEnv(t *testing.T) (*acceptanceEnv, *arrowSocket, string) {
	t.Helper()

	// A socket path must stay under the sockaddr_un limit, which t.TempDir's
	// long per-test name can exceed.
	runDir, err := os.MkdirTemp("", "qui")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(runDir) })

	eng := surface.New(runDir)
	socket, err := eng.Prepare(domain.Namespace(uiNamespace))
	require.NoError(t, err)
	arrow := startArrowSocket(t, socket)

	env := newAcceptanceEnv(t, func(c *app.Container) {
		c.Surface = usecases.NewSurfaceUsecase(listenRuntime{}, eng)
	})
	env.v0.AuthGate.SetRequired(true)

	code, _, err := env.app.Auth.GeneratePairingCode(t.Context())
	require.NoError(t, err)
	token, err := env.app.Auth.Redeem(t.Context(), code, "dev-1", "laptop")
	require.NoError(t, err)

	return env, arrow, token
}

func uiGet(t *testing.T, env *acceptanceEnv, token, path string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, env.srv.URL+path, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Cookie", "session=secret")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestAcceptance_UIRouteSitsBehindTheBearerGate(t *testing.T) {
	env := newAcceptanceEnv(t, func(c *app.Container) {
		c.Surface = usecases.NewSurfaceUsecase(idleRuntime{}, surface.New(t.TempDir()))
	})
	env.v0.AuthGate.SetRequired(true)

	code, _, err := env.app.Auth.GeneratePairingCode(t.Context())
	require.NoError(t, err)
	token, err := env.app.Auth.Redeem(t.Context(), code, "dev-1", "laptop")
	require.NoError(t, err)

	status, _ := uiGet(t, env, "", "/v0/ui/a%2Fb/")
	assert.Equal(t, http.StatusUnauthorized, status)
	status, _ = uiGet(t, env, "wrong", "/v0/ui/a%2Fb/x")
	assert.Equal(t, http.StatusUnauthorized, status)

	status, body := uiGet(t, env, token, "/v0/ui/a%2Fb/")
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Contains(t, body, "arrow has no open surface")
}

func TestAcceptance_UIUnknownArrowIs404ForAnAuthenticatedClient(t *testing.T) {
	env := newAcceptanceEnv(t)
	env.v0.AuthGate.SetRequired(true)

	code, _, err := env.app.Auth.GeneratePairingCode(t.Context())
	require.NoError(t, err)
	token, err := env.app.Auth.Redeem(t.Context(), code, "dev-1", "laptop")
	require.NoError(t, err)

	status, _ := uiGet(t, env, token, "/v0/ui/a%2Fb/")
	assert.Equal(t, http.StatusNotFound, status)
}

func TestAcceptance_UIProxiesToTheArrowSocket(t *testing.T) {
	env, arrow, token := newUIEnv(t)

	status, body := uiGet(t, env, token, "/v0/ui/github.com%2Fquiver%2Fchat/assets/app.js?x=1&y=a%2Fb")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "hello from the arrow", body)

	auth, cookie, path, query := arrow.seen()
	assert.Empty(t, auth, "the bearer token must not reach the arrow")
	assert.Empty(t, cookie, "cookies must not reach the arrow")
	assert.Equal(t, "/assets/app.js", path)
	assert.Equal(t, "x=1&y=a%2Fb", query)
}

func TestAcceptance_UIWebSocketEchoThroughTheRoute(t *testing.T) {
	env, _, token := newUIEnv(t)

	wsURL := "ws" + env.srv.URL[len("http"):] + "/v0/ui/github.com%2Fquiver%2Fchat/ws"

	_, denied, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err, "an unauthenticated upgrade is refused")
	require.Equal(t, http.StatusUnauthorized, denied.StatusCode)
	_ = denied.Body.Close()

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Authorization": {"Bearer " + token}})
	require.NoError(t, err)
	defer resp.Body.Close()
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ping")))
	typ, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, typ)
	assert.Equal(t, "ping", string(msg))
}
