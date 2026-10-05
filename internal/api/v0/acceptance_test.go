package v0_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	apiv0 "github.com/rabbytesoftware/quiver.core/internal/api/v0"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	wshandler "github.com/rabbytesoftware/quiver.core/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

const (
	acceptanceQuery  = "chrom"
	acceptanceBareNS = "github.com/quiver/chromatic"
	acceptanceRef    = "v1.4.0"
	acceptanceBranch = "main"
	acceptanceCommit = "0123456789abcdef0123456789abcdef01234567"
)

// acceptanceManifest is the bytes discovery writes to the vault and the add
// path later parses back out of it. It is never fetched twice — that is the
// property this file exists to prove.
const acceptanceManifest = "name: Chromatic\n"

func acceptanceArrow() *domain.Arrow {
	return &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        "Chromatic",
			Description: "A chromatic browser",
			Tags:        []string{"browser"},
			Media:       domain.ArrowMedia{Icon: "icon.png"},
		},
		Targets: map[domain.OS]domain.Target{domain.CurrentOS(): {}},
	}
}

// countingManifold answers from a fixed manifest and counts every network
// resolve. ParseArrow is local work on bytes already held, so it is counted
// separately: only a resolve is a request to the host.
type countingManifold struct {
	mu       sync.Mutex
	resolves int
	parses   int
}

func (m *countingManifold) ResolveArrow(
	_ context.Context,
	ns domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	m.mu.Lock()
	m.resolves++
	m.mu.Unlock()

	if ns.BareNamespace() != domain.Namespace(acceptanceBareNS) {
		return nil, nil, "", fmt.Errorf("manifold: no manifest for %s", ns)
	}
	return acceptanceArrow(), []byte(acceptanceManifest), "ARROW.md", nil
}

func (m *countingManifold) ResolveArrowAt(
	context.Context,
	domain.Namespace,
	string,
) (*domain.Arrow, []byte, string, error) {
	return nil, nil, "", fmt.Errorf("manifold: resolve arrow at not used")
}

func (m *countingManifold) ParseArrow(
	[]byte,
) (*domain.Arrow, error) {
	m.mu.Lock()
	m.parses++
	m.mu.Unlock()
	return acceptanceArrow(), nil
}

func (m *countingManifold) ResolveCollection(
	context.Context,
	domain.Namespace,
) (*domain.Collection, error) {
	return nil, fmt.Errorf("manifold: collections not used")
}

func (m *countingManifold) ParseCollection(
	[]byte,
	domain.Namespace,
) (*domain.Collection, error) {
	return nil, fmt.Errorf("manifold: collections not used")
}

func (m *countingManifold) ListChannels(
	context.Context,
	domain.Namespace,
) ([]manifold.ChannelInfo, error) {
	return nil, fmt.Errorf("manifold: list channels not used")
}

func (m *countingManifold) Snapshot(
	context.Context,
	domain.Namespace,
) (domain.RefSnapshot, error) {
	return domain.RefSnapshot{
		Branches: map[string]string{acceptanceBranch: acceptanceCommit},
		Head:     acceptanceBranch,
	}, nil
}

func (m *countingManifold) FreshSnapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	return m.Snapshot(ctx, ns)
}

func (m *countingManifold) ResolveArrowAtCommit(
	ctx context.Context,
	ns domain.Namespace,
	_ string,
	commit string,
) (*domain.Arrow, []byte, string, error) {
	if commit != acceptanceCommit {
		return nil, nil, "", fmt.Errorf("manifold: no manifest at %s", commit)
	}
	arrow, raw, filename, err := m.ResolveArrow(ctx, ns)
	if err != nil {
		return nil, nil, "", err
	}
	arrow.Namespace = ns
	return arrow, raw, filename, nil
}

func (m *countingManifold) counts() (resolves, parses int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resolves, m.parses
}

// blockingProvider answers one candidate, but not until the test releases it.
// Holding the pass open is what makes subscribing to its stream deterministic:
// nothing can be verified before there is a subscriber to receive it.
type blockingProvider struct {
	release chan struct{}

	mu      sync.Mutex
	queries []string
}

func (p *blockingProvider) Host() string { return "github.com" }

func (p *blockingProvider) CanSearch() bool { return true }

func (p *blockingProvider) Search(
	ctx context.Context,
	req provider.SearchRequest,
) ([]provider.Candidate, error) {
	p.mu.Lock()
	p.queries = append(p.queries, req.Text)
	p.mu.Unlock()

	select {
	case <-p.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return []provider.Candidate{{
		Namespace:     domain.Namespace(acceptanceBareNS),
		Name:          "chromatic",
		Description:   "a browser",
		Stars:         128,
		Source:        "github.com",
		DefaultBranch: acceptanceBranch,
	}}, nil
}

func (p *blockingProvider) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", errNotProviderSearch
}

func (p *blockingProvider) BlobFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (p *blockingProvider) OwnerAvatarURL(
	_ domain.Namespace,
) string {
	return ""
}

func (p *blockingProvider) RepoPageURL(
	_ domain.Namespace,
) string {
	return ""
}

func (p *blockingProvider) RepoMetadata(
	_ context.Context,
	_ domain.Namespace,
) (domain.RepoMetadata, error) {
	return domain.RepoMetadata{}, nil
}

func (p *blockingProvider) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]domain.ReleaseAsset, error) {
	return nil, nil
}

func (p *blockingProvider) DefaultBranches() []string { return nil }

var errNotProviderSearch = errors.New("stub provider: discovery never asks this")

func (p *blockingProvider) searches() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queries)
}

// acceptanceEnv is the whole daemon: real SQLite, a real vault on a temp home,
// and stubs only where the network would otherwise be.
type acceptanceEnv struct {
	srv      *httptest.Server
	app      *app.Container
	v0       *apiv0.Container
	ws       *wshandler.Handler
	provider *blockingProvider
	manifold *countingManifold
}

// stop runs one layer's shutdown under a bound, so a drain that wedges fails the
// test instead of hanging it.
func stop(shutdown func(ctx context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = shutdown(ctx)
}

// newAcceptanceEnv builds the daemon. Each before hook runs on the app
// container ahead of the v0 layer, so a test can swap a piece for a stub.
func newAcceptanceEnv(
	t *testing.T,
	before ...func(*app.Container),
) *acceptanceEnv {
	t.Helper()

	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { stop(engines.Shutdown) })

	man := &countingManifold{}
	prov := &blockingProvider{release: make(chan struct{})}
	engines.Manifold = man
	engines.Providers = []provider.Provider{prov}

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	appContainer, err := app.New(engines, adapters, app.WithHomeDir(home))
	require.NoError(t, err)
	require.NotNil(t, appContainer.Discovery, "discovery must be wired when a vault and a manifold exist")

	// This env is the daemon, so it is torn down like the daemon: every database
	// it opened has to be closed before t.TempDir removes the tree under it.
	// Cleanups run last-registered-first, so the app drains here, ahead of the
	// adapter and engine handles registered above it — those two own disjoint
	// files, so only draining first is load-bearing.
	t.Cleanup(func() { stop(appContainer.Shutdown) })

	for _, hook := range before {
		hook(appContainer)
	}

	v0, err := apiv0.New(appContainer)
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	v0.Register(r.Group(v0.Prefix()))

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	handler, ok := v0.WSHandler().(*wshandler.Handler)
	require.True(t, ok)

	return &acceptanceEnv{srv: srv, app: appContainer, v0: v0, ws: handler, provider: prov, manifold: man}
}

func (e *acceptanceEnv) get(
	t *testing.T,
	path string,
) (int, []byte) {
	t.Helper()

	resp, err := http.Get(e.srv.URL + path)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

func (e *acceptanceEnv) postJSON(
	t *testing.T,
	path string,
	body string,
) (int, []byte) {
	t.Helper()

	resp, err := http.Post(e.srv.URL+path, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, got
}

func decodeInto(
	t *testing.T,
	body []byte,
	data any,
) {
	t.Helper()

	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env), "body: %s", body)
	require.True(t, env.Success, "body: %s", body)
	require.NoError(t, json.Unmarshal(env.Data, data))
}

// TestAcceptance_SearchDiscoverStreamAddSearch is the branch's acceptance test.
// It walks the whole lifecycle — search, find, add — and asserts the payoff
// explicitly: adding a discovered arrow costs no second request, because
// discovery already proved and cached its manifest.
func TestAcceptance_SearchDiscoverStreamAddSearch(t *testing.T) {
	env := newAcceptanceEnv(t)

	// 1. A fresh install knows nothing.
	status, body := env.get(t, "/v0/search?q="+acceptanceQuery)
	require.Equal(t, http.StatusOK, status)

	var local []apidto.SearchResultDTO
	decodeInto(t, body, &local)
	assert.Empty(t, local, "a fresh install has nothing to answer with")

	// 2. Discovery starts and answers immediately, without waiting for the
	//    provider that is still blocked.
	status, body = env.postJSON(t, "/v0/search/discover", `{"q":"`+acceptanceQuery+`"}`)
	require.Equal(t, http.StatusAccepted, status)

	var started apidto.DiscoveryJobStartedDTO
	decodeInto(t, body, &started)
	require.NotEmpty(t, started.JobID)
	assert.Equal(t, acceptanceQuery, started.Query)

	// 3. Subscribe, then let the provider answer, and receive verified results.
	conn := dialJob(t, env.srv, started.JobID)
	env.ws.Discovery.WaitRegistered()
	close(env.provider.release)

	streamed := readResult(t, conn)
	assert.Equal(t, acceptanceBareNS, streamed.Namespace)
	assert.Equal(t, "Chromatic", streamed.Name)
	assert.Equal(t, []string{acceptanceBranch}, streamed.Versions)
	assert.Equal(t, models.ProvenanceSeen, streamed.Provenance)
	assert.Equal(t, 128, streamed.Stars)
	assert.False(t, streamed.Installed)

	// 4. The socket closes and the summary is read once.
	require.NoError(t, conn.Close())

	summary := waitForSummary(t, env, started.JobID)
	assert.Equal(t, 1, summary.Found)
	assert.Equal(t, 1, summary.Verified)
	assert.Zero(t, summary.Skipped)
	require.Len(t, summary.Providers, 2, "one outcome per pass")
	tagged := summary.Providers[0]
	assert.Equal(t, "github.com", tagged.Host)
	assert.Equal(t, "tagged", tagged.Pass)
	assert.True(t, tagged.OK)
	assert.Equal(t, 1, tagged.Returned)
	unmarked := summary.Providers[1]
	assert.Equal(t, "github.com", unmarked.Host)
	assert.Equal(t, "unmarked", unmarked.Pass)

	resolvesAfterDiscovery, _ := env.manifold.counts()
	require.Equal(t, 1, resolvesAfterDiscovery, "discovery proves each candidate exactly once")
	require.Equal(t, 2, env.provider.searches(), "one search per pass")

	// 5. Adding the discovered arrow fetches its manifest once more, at the
	//    exact commit the install records, and asks no provider anything.
	discoveredNS := acceptanceBareNS + "@" + acceptanceBranch
	status, body = env.postJSON(t, "/v0/arrow/"+url.PathEscape(discoveredNS), "")
	require.Equal(t, http.StatusCreated, status, "body: %s", body)

	resolvesAfterAdd, _ := env.manifold.counts()
	assert.Equal(t, resolvesAfterDiscovery+1, resolvesAfterAdd,
		"add fetches the manifest exactly once, at the commit it installs")
	assert.Equal(t, 2, env.provider.searches(),
		"add must not ask any provider anything")

	// 6. The arrow is now a local result. The add is accepted before the
	//    catalog projection has run, so until it does the same arrow answers
	//    from the vault index as a merely seen result — correct, just not yet
	//    the whole truth.
	require.Eventually(t, func() bool {
		st, b := env.get(t, "/v0/search?q="+acceptanceQuery)
		if st != http.StatusOK {
			return false
		}
		var got []apidto.SearchResultDTO
		decodeInto(t, b, &got)
		if len(got) != 1 || !got[0].Installed {
			return false
		}
		local = got
		return true
	}, 5*time.Second, 10*time.Millisecond)
	require.Len(t, local, 1)
	assert.Equal(t, acceptanceBareNS, local[0].Namespace)
	assert.Equal(t, "Chromatic", local[0].Name)
	assert.True(t, local[0].Installed)
	assert.Equal(t, models.ProvenanceInstalled, local[0].Provenance)

	// 7. The added row reads back in the selector model's wire shape.
	assertAddedWireShape(t, env, discoveredNS)

	// Nothing after the add reached a provider or fetched a manifest again.
	finalResolves, _ := env.manifold.counts()
	assert.Equal(t, resolvesAfterAdd, finalResolves)
	assert.Equal(t, 2, env.provider.searches())
}

// assertAddedWireShape reads the detail, list and manifest of an arrow added
// from a branch selector and checks each carries the selector model's fields
// and none of the removed ones.
func assertAddedWireShape(
	t *testing.T,
	env *acceptanceEnv,
	ns string,
) {
	t.Helper()

	var detail map[string]any
	require.Eventually(t, func() bool {
		st, b := env.get(t, "/v0/arrow/"+url.PathEscape(ns))
		if st != http.StatusOK {
			return false
		}
		decodeInto(t, b, &detail)
		return detail["resolved_ref"] != ""
	}, 5*time.Second, 10*time.Millisecond)

	assert.Equal(t, "channel", detail["selector_kind"])
	assert.Equal(t, acceptanceBranch, detail["resolved_ref"])
	assert.Equal(t, acceptanceCommit, detail["installed_commit"])
	assert.NotContains(t, detail, "available")
	assert.Equal(t, false, detail["outdated"])
	for _, removed := range []string{"channel", "installed_constraint", "recommended_ref", "installed_ref"} {
		assert.NotContains(t, detail, removed)
	}

	status, body := env.get(t, "/v0/arrow")
	require.Equal(t, http.StatusOK, status)
	var list []map[string]any
	decodeInto(t, body, &list)
	require.Len(t, list, 1)
	versions, ok := list[0]["versions"].([]any)
	require.True(t, ok)
	require.Len(t, versions, 1)
	version, ok := versions[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, acceptanceBranch, version["ref"])
	assert.Equal(t, acceptanceBranch, version["resolved_ref"])
	assert.NotContains(t, version, "constraint")

	status, body = env.get(t, "/v0/arrow/"+url.PathEscape(ns)+"/manifest")
	require.Equal(t, http.StatusOK, status)
	var manifest struct {
		Manifest map[string]any `json:"manifest"`
	}
	decodeInto(t, body, &manifest)
	assert.Contains(t, manifest.Manifest, "metadata")
	for _, key := range []string{"selector_kind", "resolved", "available", "installed_at", "user_installed"} {
		assert.NotContains(t, manifest.Manifest, key)
	}
}

func dialJob(
	t *testing.T,
	srv *httptest.Server,
	jobID string,
) *websocket.Conn {
	t.Helper()

	target := "ws" + srv.URL[len("http"):] + "/v0/search/discover/" + jobID
	conn, _, err := websocket.DefaultDialer.Dial(target, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readResult(
	t *testing.T,
	conn *websocket.Conn,
) apidto.SearchResultDTO {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var got apidto.SearchResultDTO
	require.NoError(t, json.Unmarshal(msg, &got))
	return got
}

// waitForSummary reads the job until the pass has finished. The client reads it
// once for real; the loop is here only because a test has no other way to know
// the pass ended.
func waitForSummary(
	t *testing.T,
	env *acceptanceEnv,
	jobID string,
) apidto.DiscoveryJobDTO {
	t.Helper()

	var summary apidto.DiscoveryJobDTO
	require.Eventually(t, func() bool {
		status, body := env.get(t, "/v0/search/discover/"+jobID)
		if status != http.StatusOK {
			return false
		}
		var got apidto.DiscoveryJobDTO
		decodeInto(t, body, &got)
		if got.Status != string(usecases.JobCompleted) {
			return false
		}
		summary = got
		return true
	}, 5*time.Second, 10*time.Millisecond)

	assert.Equal(t, jobID, summary.JobID)
	assert.Equal(t, acceptanceQuery, summary.Query)
	return summary
}

const (
	uiNamespace = "github.com/quiver/chat"
	// uiResolved is the catalogued ref the runtime reports for the bare
	// namespace, and the one the arrow's socket is prepared under.
	uiResolved = uiNamespace + "@stable"
)

// listenRuntime resolves any namespace to uiResolved, with a listen
// surface open.
type listenRuntime struct {
	usecases.RuntimeUsecase
}

func (listenRuntime) GetRuntime(
	context.Context,
	domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
	return &domainRuntime.ArrowRuntime{
		Ref: uiResolved,
		Execution: &domainRuntime.Execution{ID: "e1", Surface: &domainRuntime.Surface{
			Mode: domainRuntime.SurfaceModeListen, Path: "/",
		}},
	}, nil
}

// runLifetimeRuntime reports the surface a real wizard execution opens and
// closes, the way the runtime repository's drain records it on the aggregate.
type runLifetimeRuntime struct {
	usecases.RuntimeUsecase
	mu      sync.Mutex
	surface *domainRuntime.Surface
}

func (r *runLifetimeRuntime) GetRuntime(
	context.Context,
	domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt := &domainRuntime.ArrowRuntime{Ref: uiResolved, Execution: &domainRuntime.Execution{ID: "e1"}}
	if r.surface != nil {
		surface := *r.surface
		rt.Execution.Surface = &surface
	}
	return rt, nil
}

func (r *runLifetimeRuntime) drain(
	exec wizard.Execution,
) {
	for evt := range exec.Events() {
		r.mu.Lock()
		if evt.Kind == wizard.EventKindSurface {
			r.surface = evt.Surface
		}
		if evt.Kind == wizard.EventKindSurfaceClosed {
			r.surface = nil
		}
		r.mu.Unlock()
	}
}

// idleRuntime knows every arrow and has nothing running for any of them.
type idleRuntime struct {
	usecases.RuntimeUsecase
}

func (idleRuntime) GetRuntime(
	_ context.Context,
	ns domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
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

func startArrowSocket(
	t *testing.T,
	socket string,
) *arrowSocket {
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
	env, arrow, token, _ := newUIEnvWith(t, listenRuntime{})
	return env, arrow, token
}

func newUIEnvWith(
	t *testing.T,
	rt usecases.RuntimeUsecase,
) (*acceptanceEnv, *arrowSocket, string, string) {
	t.Helper()

	// A socket path must stay under the sockaddr_un limit, which t.TempDir's
	// long per-test name can exceed.
	runDir, err := os.MkdirTemp("", "qui")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(runDir) })

	eng := surface.New(runDir)
	socket, err := eng.Prepare(domain.Namespace(uiResolved))
	require.NoError(t, err)
	arrow := startArrowSocket(t, socket)

	env := newAcceptanceEnv(t, func(c *app.Container) {
		c.Surface = usecases.NewSurfaceUsecase(rt, eng)
	})
	env.v0.AuthGate.SetRequired(true)

	code, _, err := env.app.Auth.GeneratePairingCode(t.Context())
	require.NoError(t, err)
	token, err := env.app.Auth.Redeem(t.Context(), code, "dev-1", "laptop")
	require.NoError(t, err)

	return env, arrow, token, socket
}

func waitForFileCommand(
	path string,
) string {
	if runtime.GOOS == "windows" {
		return `powershell -NoProfile -NonInteractive -Command "while (-not (Test-Path -LiteralPath '` + path + `')) { Start-Sleep -Milliseconds 20 }"`
	}
	return "while [ ! -f '" + path + "' ]; do sleep 0.02; done"
}

func createFileCommand(
	path string,
) string {
	if runtime.GOOS == "windows" {
		return `type nul > "` + path + `"`
	}
	return "touch '" + path + "'"
}

func uiGet(
	t *testing.T,
	env *acceptanceEnv,
	token string,
	path string,
) (int, string) {
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

func TestAcceptance_UIServedWhileTheRunLivesThenGoneAndTheMethodContinues(t *testing.T) {
	rt := &runLifetimeRuntime{}
	env, _, token, socket := newUIEnvWith(t, rt)

	workDir := t.TempDir()
	release := filepath.Join(workDir, "release")
	after := filepath.Join(workDir, "after")
	w, err := wizard.New(nil, 1<<20)
	require.NoError(t, err)
	t.Cleanup(func() { stop(w.Shutdown) })

	exec := w.Start(t.Context(), wizard.RunRequest{
		Namespace: domain.Namespace(uiResolved),
		Method:    domain.MethodExecute,
		Variables: map[string]string{domain.VarArrowUIListen: socket},
		WorkDir:   workDir,
		Steps: []step.Step{
			step.NewRunStep("serve", waitForFileCommand(release), false, "30s", true).
				WithUI(step.UIOptions{Title: "Chat"}),
			step.NewRunStep("after", createFileCommand(after), false, "30s", true),
		},
	})
	drained := make(chan struct{})
	go func() { defer close(drained); rt.drain(exec) }()

	require.Eventually(t, func() bool {
		status, _ := uiGet(t, env, token, "/v0/ui/github.com%2Fquiver%2Fchat/")
		return status == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond, "the surface is served while the run lives")

	require.NoError(t, os.WriteFile(release, nil, 0o600))
	<-drained

	status, body := uiGet(t, env, token, "/v0/ui/github.com%2Fquiver%2Fchat/")
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Contains(t, body, "arrow has no open surface")
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, exec.Outcome())
	assert.FileExists(t, after, "the method continues once the run exits")
}
