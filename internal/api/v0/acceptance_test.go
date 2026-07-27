package v0_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/rabbytesoftware/quiver.core/internal/engine"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

const (
	acceptanceQuery  = "chrom"
	acceptanceBareNS = "github.com/quiver/chromatic"
	acceptanceRef    = "v1.4.0"
	acceptanceBranch = "main"
)

// acceptanceManifest is the bytes discovery writes to the vault and the add
// path later parses back out of it. It is never fetched twice — that is the
// property this file exists to prove.
const acceptanceManifest = "name: Chromatic\nversion: v1.4.0\n"

func acceptanceArrow() *domain.Arrow {
	return &domain.Arrow{
		Namespace: domain.Namespace(acceptanceBareNS + "@" + acceptanceRef),
		ArrowMeta: domain.ArrowMeta{
			Name:        "Chromatic",
			Description: "A chromatic browser",
			Version:     acceptanceRef,
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

func (m *countingManifold) ResolveConstraint(
	context.Context,
	domain.Namespace,
	string,
) (string, error) {
	return acceptanceRef, nil
}

func (m *countingManifold) ResolveLatestStable(
	context.Context,
	domain.Namespace,
) (string, error) {
	return acceptanceRef, nil
}

func (m *countingManifold) ResolveDefaultBranch(
	context.Context,
	domain.Namespace,
) (string, error) {
	return "", fmt.Errorf("manifold: default branch not used")
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

func (p *blockingProvider) searches() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queries)
}

// acceptanceEnv is the whole daemon: real SQLite, a real vault on a temp home,
// and stubs only where the network would otherwise be.
type acceptanceEnv struct {
	srv      *httptest.Server
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

func newAcceptanceEnv(t *testing.T) *acceptanceEnv {
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

	return &acceptanceEnv{srv: srv, ws: handler, provider: prov, manifold: man}
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
	require.Len(t, summary.Providers, 1)
	assert.Equal(t, "github.com", summary.Providers[0].Host)
	assert.True(t, summary.Providers[0].OK)
	assert.Equal(t, 1, summary.Providers[0].Returned)

	resolvesAfterDiscovery, _ := env.manifold.counts()
	require.Equal(t, 1, resolvesAfterDiscovery, "discovery proves each candidate exactly once")
	require.Equal(t, 1, env.provider.searches())

	// 5. Adding the discovered arrow serves from the warm vault cache. This is
	//    the payoff and the assertion that matters: not that the add succeeded,
	//    but that it cost nothing.
	discoveredNS := acceptanceBareNS + "@" + acceptanceBranch
	status, body = env.postJSON(t, "/v0/arrow/"+url.PathEscape(discoveredNS), "")
	require.Equal(t, http.StatusCreated, status, "body: %s", body)

	resolvesAfterAdd, parsesAfterAdd := env.manifold.counts()
	assert.Equal(t, resolvesAfterDiscovery, resolvesAfterAdd,
		"add must not resolve again — discovery already cached the manifest")
	assert.Equal(t, 1, env.provider.searches(),
		"add must not ask any provider anything")
	assert.Positive(t, parsesAfterAdd,
		"the add path read the cached bytes rather than fetching them")

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

	// Nothing above reached a provider or the network a second time.
	finalResolves, _ := env.manifold.counts()
	assert.Equal(t, 1, finalResolves)
	assert.Equal(t, 1, env.provider.searches())
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
