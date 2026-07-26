//go:build integration

package kit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	"github.com/rabbytesoftware/quiver.core/internal/api"
	apiv0 "github.com/rabbytesoftware/quiver.core/internal/api/v0"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// Env is a fully wired test server.
type Env struct {
	URL                   string
	Home                  string
	Vault                 vault.Vault
	socketPath            string
	closeOnce             sync.Once
	closeFn               func()
	closeWithoutKillingFn func()
	closeCrashingFn       func()
	states                *stateWatcher
	arrows                *arrowWatcher
	catalog               *catalogWatcher
	collections           *collectionWatcher
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
	return NewClient(t, e.URL, e.socketPath)
}

// HTTPClient returns a bare *http.Client that routes through the Unix socket.
// Use this only when you need concurrent requests without t.Fatal integration
// (e.g., benchmark goroutines). For normal test code, prefer Client or TypedClient.
func (e *Env) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", e.socketPath)
			},
		},
		Timeout: timeout,
	}
}

// TypedClient returns a typed HTTP client pointed at this Env.
func (e *Env) TypedClient(t *testing.T) *TypedClient {
	return NewTypedClient(t, e.URL, e.socketPath)
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

// WaitForCatalogLen blocks until wantLen distinct arrows have appeared in the
// catalog stream (all arrows, regardless of user_installed) or timeout elapses.
func (e *Env) WaitForCatalogLen(t *testing.T, wantLen int, timeout time.Duration) {
	t.Helper()
	e.catalog.WaitForCount(t, wantLen, timeout)
}

// WaitForCollectionFollowed blocks until ns is reported as followed=true on the
// collection WebSocket stream. Because store.Save is registered before
// BroadcastCollection in the projection chain, the REST list is already updated
// by the time this returns.
func (e *Env) WaitForCollectionFollowed(t *testing.T, ns string, timeout time.Duration) {
	t.Helper()
	e.collections.WaitForFollowed(t, ns, timeout)
}

// BuildEnv wires a full test server using the given homeDir for path isolation.
// It registers e.Close via t.Cleanup so explicit Close calls are optional.
func BuildEnv(t *testing.T, arrowRepos *FixtureRepos, collectionRepos *FixtureRepos, home string) *Env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- cancel is called inside closeFn, which is invoked by e.Close()

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	rsv := newTestResolver(arrowRepos, collectionRepos)
	engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	appContainer, err := app.New(engines, adapters, app.WithHomeDir(home))
	require.NoError(t, err)

	v0Container, err := apiv0.New(appContainer)
	require.NoError(t, err)

	apiContainer, err := api.New(appContainer.Hub, api.BuildInfo{}, v0Container)
	require.NoError(t, err)

	// Short path required: macOS enforces UNIX_PATH_MAX = 104 chars.
	f, err := os.CreateTemp("", "qv-test-*.sock")
	require.NoError(t, err)
	socketPath := f.Name()
	f.Close()
	os.Remove(socketPath)
	t.Cleanup(func() { os.Remove(socketPath) })

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	baseURL := "http://localhost"
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		apiContainer.Run(ln) //nolint:errcheck
	}()

	states := newStateWatcher(t, baseURL, socketPath)
	arrows := newArrowWatcher(t, baseURL, socketPath)
	catalog := newCatalogWatcher(t, baseURL, socketPath)
	collections := newCollectionWatcher(t, baseURL, socketPath)
	appContainer.Start(ctx)
	closeHTTP := func() {
		// Shutdown MUST run before closing watchers — it terminates the WebSocket
		// connections that the watcher readLoops are blocked on.
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		apiContainer.Shutdown(shutdownCtx) //nolint:errcheck
		<-runDone
		states.close()
		arrows.close()
		catalog.close()
		collections.close()
	}
	e := &Env{
		URL:         baseURL,
		Home:        home,
		Vault:       engines.Vault,
		socketPath:  socketPath,
		states:      states,
		arrows:      arrows,
		catalog:     catalog,
		collections: collections,
		// Graceful shutdown, in the same order as internal.Container.Shutdown:
		// cancel processes, drain every aggregate, then release the handles.
		closeFn: func() {
			closeHTTP()
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = appContainer.Shutdown(shutdownCtx)
			_ = engines.Shutdown(shutdownCtx)
			_ = adapters.Close()
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
	return BuildEnv(s.T(), s.Repos, s.CollectionRepos, s.T().TempDir())
}

// NewEnvWithHome creates an Env using an explicit home directory.
// Used for restart-survival tests that need two envs pointing at the same storage.
func (s *IntegrationSuite) NewEnvWithHome(home string) *Env {
	return BuildEnv(s.T(), s.Repos, s.CollectionRepos, home)
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

func newStateWatcher(t *testing.T, baseURL, socketPath string) *stateWatcher {
	t.Helper()
	w := &stateWatcher{
		current:    make(map[string]string),
		currentPID: make(map[string]int),
		done:       make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/runtime"
	conn, _, err := unixWSDialer(socketPath).Dial(wsURL, nil)
	require.NoError(t, err)
	go w.readLoop(conn)
	return w
}

func unixWSDialer(socketPath string) *websocket.Dialer {
	return &websocket.Dialer{
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}
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

func newArrowWatcher(t *testing.T, baseURL, socketPath string) *arrowWatcher {
	t.Helper()
	w := &arrowWatcher{
		seen: make(map[string]struct{}),
		done: make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/arrow"
	conn, _, err := unixWSDialer(socketPath).Dial(wsURL, nil)
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
	mu        sync.Mutex
	seen      map[string]struct{}        // bare namespaces of any arrows seen
	subs      map[string][]chan struct{} // namespace-specific subscribers
	countSubs []chan struct{}            // notified on every catalog event
	done      chan struct{}
}

func newCatalogWatcher(t *testing.T, baseURL, socketPath string) *catalogWatcher {
	t.Helper()
	w := &catalogWatcher{
		seen: make(map[string]struct{}),
		subs: make(map[string][]chan struct{}),
		done: make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/arrow"
	conn, _, err := unixWSDialer(socketPath).Dial(wsURL, nil)
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
			for _, ch := range w.countSubs {
				select {
				case ch <- struct{}{}:
				default:
				}
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

// WaitForCount blocks until wantLen distinct arrows have appeared in the catalog stream or timeout elapses.
func (w *catalogWatcher) WaitForCount(
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
	w.countSubs = append(w.countSubs, notify)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		for i, ch := range w.countSubs {
			if ch == notify {
				w.countSubs = append(w.countSubs[:i], w.countSubs[i+1:]...)
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
			t.Fatalf("WaitForCatalogLen: timeout waiting for %d arrows in catalog (have %d)", wantLen, count)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForCatalogLen: timeout waiting for %d arrows in catalog (have %d)", wantLen, count)
			return
		}
	}
}

func (w *catalogWatcher) close() {
	close(w.done)
}

// collectionEvent mirrors the relevant fields from QuiverDTO pushed by GET /v0/collection WebSocket.
type collectionEvent struct {
	Namespace string `json:"namespace"`
	Followed  bool   `json:"followed"`
}

// collectionWatcher subscribes to GET /v0/collection (global stream) and tracks
// the followed state per namespace. WaitForFollowed fires as soon as the
// projection pushes followed=true — by which point the REST store is already updated.
type collectionWatcher struct {
	mu       sync.Mutex
	followed map[string]bool
	subs     map[string][]chan struct{}
	done     chan struct{}
}

func newCollectionWatcher(t *testing.T, baseURL, socketPath string) *collectionWatcher {
	t.Helper()
	w := &collectionWatcher{
		followed: make(map[string]bool),
		subs:     make(map[string][]chan struct{}),
		done:     make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/collection"
	conn, _, err := unixWSDialer(socketPath).Dial(wsURL, nil)
	require.NoError(t, err)
	go w.readLoop(conn)
	return w
}

func (w *collectionWatcher) readLoop(conn *websocket.Conn) {
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
		var evt collectionEvent
		if json.Unmarshal(msg, &evt) != nil || evt.Namespace == "" {
			continue
		}
		w.mu.Lock()
		w.followed[evt.Namespace] = evt.Followed
		var notify []chan struct{}
		if evt.Followed {
			notify = w.subs[evt.Namespace]
			delete(w.subs, evt.Namespace)
		}
		w.mu.Unlock()
		for _, ch := range notify {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

// WaitForFollowed blocks until ns is seen with followed=true or timeout elapses.
func (w *collectionWatcher) WaitForFollowed(
	t *testing.T,
	ns string,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	w.mu.Lock()
	if w.followed[ns] {
		w.mu.Unlock()
		return
	}
	w.subs[ns] = append(w.subs[ns], notify)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		if subs, ok := w.subs[ns]; ok {
			for i, ch := range subs {
				if ch == notify {
					w.subs[ns] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
			if len(w.subs[ns]) == 0 {
				delete(w.subs, ns)
			}
		}
		w.mu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	for {
		w.mu.Lock()
		f := w.followed[ns]
		w.mu.Unlock()
		if f {
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("WaitForCollectionFollowed(%s): timeout", ns)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForCollectionFollowed(%s): timeout", ns)
			return
		}
	}
}

func (w *collectionWatcher) close() {
	close(w.done)
}
