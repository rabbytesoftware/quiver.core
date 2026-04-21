//go:build integration

package kit

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/stretchr/testify/require"
)

// Env is a fully wired test server.
type Env struct {
	URL       string
	closeOnce sync.Once
	closeFn   func()
	states    *stateWatcher
	arrows    *arrowWatcher
}

// Close tears down the test server idempotently.
// It is safe to call multiple times (e.g., manually for ordered teardown and via t.Cleanup).
func (e *Env) Close() {
	e.closeOnce.Do(e.closeFn)
}

// Client returns a raw HTTP client pointed at this Env.
func (e *Env) Client(t *testing.T) *Client {
	return NewClient(t, e.URL)
}

// TypedClient returns a typed HTTP client pointed at this Env.
func (e *Env) TypedClient(t *testing.T) *TypedClient {
	return NewTypedClient(t, e.URL)
}

// WaitForState blocks until ns reaches want state or timeout elapses.
// Uses the global WebSocket runtime stream — no polling, no timing dependency.
func (e *Env) WaitForState(t *testing.T, ns string, want domain.ArrowState, timeout time.Duration) {
	t.Helper()
	e.states.WaitFor(t, ns, want, timeout)
}

// WaitForListLen blocks until wantLen distinct user-installed arrows appear in the catalog stream.
// Uses the global WebSocket catalog stream — no polling, no timing dependency.
func (e *Env) WaitForListLen(t *testing.T, wantLen int, timeout time.Duration) {
	t.Helper()
	e.arrows.WaitForCount(t, wantLen, timeout)
}

// BuildEnv wires a full test server using the given homeDir for path isolation.
// It registers e.Close via t.Cleanup so explicit Close calls are optional.
func BuildEnv(t *testing.T, repos FixtureRepos, home string) *Env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- cancel is called inside closeFn, which is invoked by e.Close()

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	rsv := newTestResolver(repos)
	engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	hub := api.NewHub()
	appContainer, err := app.New(engines, adapters, hub, app.WithHomeDir(home))
	require.NoError(t, err)

	apiContainer, err := api.New(appContainer, hub)
	require.NoError(t, err)

	srv := httptest.NewServer(apiContainer)
	states := newStateWatcher(t, srv.URL)
	arrows := newArrowWatcher(t, srv.URL)
	closeFn := func() {
		srv.Close()
		cancel()
		states.close()
		arrows.close()
		// Give engine goroutines (asynx workers, process runners) time to drain
		// after context cancellation. Without this, goroutines accumulate and
		// saturate the CPU, causing progressive slowdown across sequential tests.
		time.Sleep(2 * time.Second)
	}
	e := &Env{URL: srv.URL, closeFn: closeFn, states: states, arrows: arrows}
	t.Cleanup(e.Close)
	return e
}

// NewEnv creates an Env with a fresh temp directory as its home.
func (s *IntegrationSuite) NewEnv() *Env {
	return BuildEnv(s.T(), s.Repos, s.T().TempDir())
}

// NewEnvWithHome creates an Env using an explicit home directory.
// Used for restart-survival tests that need two envs pointing at the same storage.
func (s *IntegrationSuite) NewEnvWithHome(home string) *Env {
	return BuildEnv(s.T(), s.Repos, home)
}

// runtimeEvent mirrors the relevant fields of ArrowRuntimeDTO from the WebSocket stream.
type runtimeEvent struct {
	Namespace string `json:"namespace"`
	State     string `json:"state"`
}

// stateWatcher subscribes to GET /v0/arrow.runtime (global) and stores all events.
// WaitFor checks history first so callers never miss an event that arrived before they waited.
type stateWatcher struct {
	mu      sync.Mutex
	history []runtimeEvent
	subs    []chan struct{}
	done    chan struct{}
}

func newStateWatcher(t *testing.T, baseURL string) *stateWatcher {
	t.Helper()
	w := &stateWatcher{done: make(chan struct{})}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/arrow.runtime"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	go w.readLoop(conn)
	return w
}

func (w *stateWatcher) readLoop(conn *websocket.Conn) {
	defer conn.Close()
	for {
		select {
		case <-w.done:
			return
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var evt runtimeEvent
		if json.Unmarshal(msg, &evt) == nil && evt.State != "" {
			w.mu.Lock()
			w.history = append(w.history, evt)
			subs := make([]chan struct{}, len(w.subs))
			copy(subs, w.subs)
			w.mu.Unlock()
			for _, ch := range subs {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}
}

// WaitFor blocks until ns reaches want state or timeout elapses.
// Checks history before waiting so it never misses events that already arrived.
func (w *stateWatcher) WaitFor(
	t *testing.T,
	ns string,
	want domain.ArrowState,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	w.mu.Lock()
	for _, evt := range w.history {
		if evt.Namespace == ns && domain.ArrowState(evt.State) == want {
			w.mu.Unlock()
			return
		}
	}
	w.subs = append(w.subs, notify)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		for i, ch := range w.subs {
			if ch == notify {
				w.subs = append(w.subs[:i], w.subs[i+1:]...)
				break
			}
		}
		w.mu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	for {
		w.mu.Lock()
		for _, evt := range w.history {
			if evt.Namespace == ns && domain.ArrowState(evt.State) == want {
				w.mu.Unlock()
				return
			}
		}
		w.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("WaitForState(%s): timeout waiting for %s", ns, want)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForState(%s): timeout waiting for %s", ns, want)
			return
		}
	}
}

func (w *stateWatcher) close() {
	close(w.done)
}

// arrowEvent mirrors the userInstalled field from ArrowDTO pushed by GET /v0/arrow WebSocket.
type arrowEvent struct {
	Namespace     string `json:"namespace"`
	UserInstalled bool   `json:"user_installed"`
}

// arrowWatcher subscribes to GET /v0/arrow (global catalog stream).
// WaitForCount blocks until wantLen distinct user-installed bare namespaces are seen.
type arrowWatcher struct {
	mu   sync.Mutex
	seen map[string]struct{} // bare namespaces of user-installed arrows seen so far
	subs []chan struct{}
	done chan struct{}
}

func newArrowWatcher(t *testing.T, baseURL string) *arrowWatcher {
	t.Helper()
	w := &arrowWatcher{
		seen: make(map[string]struct{}),
		done: make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/arrow"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	go w.readLoop(conn)
	return w
}

func (w *arrowWatcher) readLoop(conn *websocket.Conn) {
	defer conn.Close()
	for {
		select {
		case <-w.done:
			return
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var evt arrowEvent
		if json.Unmarshal(msg, &evt) == nil && evt.UserInstalled && evt.Namespace != "" {
			bare := evt.Namespace
			if idx := strings.LastIndex(bare, "@"); idx >= 0 {
				bare = bare[:idx]
			}
			w.mu.Lock()
			w.seen[bare] = struct{}{}
			subs := make([]chan struct{}, len(w.subs))
			copy(subs, w.subs)
			w.mu.Unlock()
			for _, ch := range subs {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}
}

// WaitForCount blocks until wantLen distinct user-installed arrows are seen or timeout elapses.
func (w *arrowWatcher) WaitForCount(
	t *testing.T,
	wantLen int,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	w.mu.Lock()
	if len(w.seen) >= wantLen {
		w.mu.Unlock()
		return
	}
	w.subs = append(w.subs, notify)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		for i, ch := range w.subs {
			if ch == notify {
				w.subs = append(w.subs[:i], w.subs[i+1:]...)
				break
			}
		}
		w.mu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	for {
		w.mu.Lock()
		count := len(w.seen)
		w.mu.Unlock()
		if count >= wantLen {
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("WaitForListLen: timeout waiting for %d user-installed arrows (have %d)", wantLen, count)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForListLen: timeout waiting for %d user-installed arrows (have %d)", wantLen, count)
			return
		}
	}
}

func (w *arrowWatcher) close() {
	close(w.done)
}
