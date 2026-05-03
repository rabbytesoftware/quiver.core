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
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	apiv0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// Env is a fully wired test server.
type Env struct {
	URL                   string
	Home                  string
	Vault                 vault.Vault
	closeOnce             sync.Once
	closeFn               func()
	closeWithoutKillingFn func()
	closeCrashingFn       func()
	states                *stateWatcher
	arrows                *arrowWatcher
	catalog               *catalogWatcher
}

// Close tears down the test server idempotently.
// It is safe to call multiple times (e.g., manually for ordered teardown and via t.Cleanup).
func (e *Env) Close() {
	e.closeOnce.Do(e.closeFn)
}

// CloseWithoutKilling shuts down the HTTP server and DB but does NOT cancel the
// execution context. OS processes (e.g. sleep 3600) spawned by running arrows
// remain alive after this call — useful for testing alive-PID crash recovery.
// It is safe to call multiple times (shares the same once guard as Close).
func (e *Env) CloseWithoutKilling() {
	e.closeOnce.Do(e.closeWithoutKillingFn)
}

// CloseCrashing simulates a dead-PID crash: OS processes are killed via context
// cancellation, but appContainer.Shutdown is never called. The event store is
// left in whatever mid-flight transient state the execution was in — useful for
// testing crash recovery of Installing, Uninstalling, Updating, etc.
// It is safe to call multiple times (shares the same once guard as Close).
func (e *Env) CloseCrashing() {
	e.closeOnce.Do(e.closeCrashingFn)
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

// WaitForActivePID blocks until a non-zero PID is recorded for ns or timeout elapses.
// Use this before CloseWithoutKilling() to ensure RecordPID has been persisted.
func (e *Env) WaitForActivePID(t *testing.T, ns string, timeout time.Duration) {
	t.Helper()
	e.states.WaitForActivePID(t, ns, timeout)
}

// WaitForListLen blocks until wantLen distinct user-installed arrows appear in the catalog stream.
// Uses the global WebSocket catalog stream — no polling, no timing dependency.
func (e *Env) WaitForListLen(t *testing.T, wantLen int, timeout time.Duration) {
	t.Helper()
	e.arrows.WaitForCount(t, wantLen, timeout)
}

// WaitForArrow blocks until ns appears in the catalog WebSocket stream (regardless of installation status).
// Uses the global WebSocket catalog stream — no polling, pure async notification.
// Suitable for waiting after Seed operations which may not mark arrows as user-installed.
func (e *Env) WaitForArrow(t *testing.T, ns string, timeout time.Duration) {
	t.Helper()
	e.catalog.WaitFor(t, ns, timeout)
}

// BuildEnv wires a full test server using the given homeDir for path isolation.
// It registers e.Close via t.Cleanup so explicit Close calls are optional.
func BuildEnv(t *testing.T, arrowRepos *FixtureRepos, quiverRepos *FixtureRepos, home string) *Env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- cancel is called inside closeFn, which is invoked by e.Close()

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	rsv := newTestResolver(arrowRepos, quiverRepos)
	engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	appContainer, err := app.New(engines, adapters, app.WithHomeDir(home))
	require.NoError(t, err)

	v0Container, err := apiv0.New(appContainer)
	require.NoError(t, err)

	apiContainer, err := api.New(appContainer.Hub, v0Container)
	require.NoError(t, err)

	srv := httptest.NewServer(apiContainer)
	states := newStateWatcher(t, srv.URL)
	arrows := newArrowWatcher(t, srv.URL)
	catalog := newCatalogWatcher(t, srv.URL)
	appContainer.Start(ctx)
	closeHTTP := func() {
		// srv.Close() MUST run before states.close(), arrows.close(), and catalog.close().
		// The watcher readLoops block on conn.ReadMessage(); srv.Close() terminates
		// those connections (causing ReadMessage to error and the goroutine to exit).
		// If the order were reversed, the readLoop goroutines would linger until
		// OS-level teardown.
		srv.Close()
		states.close()
		arrows.close()
		catalog.close()
	}
	e := &Env{
		URL:     srv.URL,
		Home:    home,
		Vault:   engines.Vault,
		states:  states,
		arrows:  arrows,
		catalog: catalog,
		// Graceful shutdown: cancel processes, then wait for app to drain.
		closeFn: func() {
			closeHTTP()
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = appContainer.Shutdown(shutdownCtx)
		},
		// Simulate alive-PID crash: skip cancel and Shutdown so OS processes survive.
		closeWithoutKillingFn: func() {
			closeHTTP()
		},
		// Simulate dead-PID crash: kill OS processes but skip graceful Shutdown,
		// leaving the event store in a mid-flight transient state for recovery tests.
		closeCrashingFn: func() {
			closeHTTP()
			cancel()
		},
	}
	t.Cleanup(e.Close)
	return e
}

// NewEnv creates an Env with a fresh temp directory as its home.
func (s *IntegrationSuite) NewEnv() *Env {
	return BuildEnv(s.T(), s.Repos, s.QuiverRepos, s.T().TempDir())
}

// NewEnvWithHome creates an Env using an explicit home directory.
// Used for restart-survival tests that need two envs pointing at the same storage.
func (s *IntegrationSuite) NewEnvWithHome(home string) *Env {
	return BuildEnv(s.T(), s.Repos, s.QuiverRepos, home)
}

// runtimeEvent mirrors the relevant fields of ArrowRuntimeDTO from the WebSocket stream.
type runtimeEvent struct {
	Namespace string           `json:"namespace"`
	State     string           `json:"state"`
	ActiveRun *activeRunFields `json:"active_run,omitempty"`
}

// activeRunFields captures PID from the active_run field of ArrowRuntimeDTO.
type activeRunFields struct {
	PID int `json:"pid"`
}

// stateWatcher subscribes to GET /v0/arrow.runtime (global) and tracks the current
// state per namespace. WaitFor checks the current state before waiting so it never
// misses an event that arrived before the call — but only matches the LATEST state,
// not any historical state. This prevents false-positive matches when a namespace
// cycles through the same state multiple times (e.g. ready → running → ready).
type stateWatcher struct {
	mu         sync.Mutex
	current    map[string]string // latest state per namespace
	currentPID map[string]int    // latest active PID per namespace
	subs       []chan struct{}
	done       chan struct{}
}

func newStateWatcher(t *testing.T, baseURL string) *stateWatcher {
	t.Helper()
	w := &stateWatcher{
		current:    make(map[string]string),
		currentPID: make(map[string]int),
		done:       make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/runtime"
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
			w.current[evt.Namespace] = evt.State
			if evt.ActiveRun != nil && evt.ActiveRun.PID > 0 {
				w.currentPID[evt.Namespace] = evt.ActiveRun.PID
			}
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

// WaitForActivePID blocks until a non-zero PID is recorded for ns or timeout elapses.
func (w *stateWatcher) WaitForActivePID(
	t *testing.T,
	ns string,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	w.mu.Lock()
	if w.currentPID[ns] > 0 {
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
		if w.currentPID[ns] > 0 {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("WaitForActivePID(%s): timeout waiting for non-zero PID", ns)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForActivePID(%s): timeout waiting for non-zero PID", ns)
			return
		}
	}
}

// WaitFor blocks until ns reaches want state or timeout elapses.
// Checks current state before waiting so it never misses an event that already arrived.
func (w *stateWatcher) WaitFor(
	t *testing.T,
	ns string,
	want domain.ArrowState,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	w.mu.Lock()
	if w.current[ns] == string(want) {
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
		if w.current[ns] == string(want) {
			w.mu.Unlock()
			return
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

// catalogWatcher subscribes to GET /v0/arrow (catalog stream) and tracks any arrow that appears,
// regardless of user_installed status. Useful for waiting after Seed operations.
type catalogWatcher struct {
	mu   sync.Mutex
	seen map[string]struct{}        // bare namespaces of any arrows seen
	subs map[string][]chan struct{} // subscribers waiting for specific namespaces
	done chan struct{}
}

func newCatalogWatcher(t *testing.T, baseURL string) *catalogWatcher {
	t.Helper()
	w := &catalogWatcher{
		seen: make(map[string]struct{}),
		subs: make(map[string][]chan struct{}),
		done: make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/arrow"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	go w.readLoop(conn)
	return w
}

func (w *catalogWatcher) readLoop(conn *websocket.Conn) {
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
		if json.Unmarshal(msg, &evt) == nil && evt.Namespace != "" {
			// Track bare namespace (strip @ref if present)
			bare := evt.Namespace
			if idx := strings.LastIndex(bare, "@"); idx >= 0 {
				bare = bare[:idx]
			}
			w.mu.Lock()
			w.seen[bare] = struct{}{}
			// Notify all subscribers waiting for this namespace
			if subs, ok := w.subs[bare]; ok {
				for _, ch := range subs {
					select {
					case ch <- struct{}{}:
					default:
					}
				}
				delete(w.subs, bare)
			}
			w.mu.Unlock()
		}
	}
}

// WaitFor blocks until ns (bare namespace) appears in the catalog stream or timeout elapses.
func (w *catalogWatcher) WaitFor(
	t *testing.T,
	ns string,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	// Strip @ref from ns if present to get bare namespace
	bare := ns
	if idx := strings.LastIndex(bare, "@"); idx >= 0 {
		bare = bare[:idx]
	}

	w.mu.Lock()
	if _, ok := w.seen[bare]; ok {
		w.mu.Unlock()
		return
	}
	w.subs[bare] = append(w.subs[bare], notify)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		if subs, ok := w.subs[bare]; ok {
			for i, ch := range subs {
				if ch == notify {
					w.subs[bare] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
			if len(w.subs[bare]) == 0 {
				delete(w.subs, bare)
			}
		}
		w.mu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	for {
		w.mu.Lock()
		_, ok := w.seen[bare]
		w.mu.Unlock()
		if ok {
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("WaitForArrow(%s): timeout waiting for arrow to appear in catalog", bare)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForArrow(%s): timeout waiting for arrow to appear in catalog", bare)
			return
		}
	}
}

func (w *catalogWatcher) close() {
	close(w.done)
}
