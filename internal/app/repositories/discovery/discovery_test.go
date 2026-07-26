package discovery_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// ─── stub provider ───────────────────────────────────────────────────────────

type stubProvider struct {
	host       string
	candidates []provider.Candidate
	err        error
	queries    chan provider.SearchRequest
}

func (s *stubProvider) Host() string { return s.host }

func (s *stubProvider) Search(
	_ context.Context,
	req provider.SearchRequest,
) ([]provider.Candidate, error) {
	if s.queries != nil {
		s.queries <- req
	}
	return s.candidates, s.err
}

// ─── stub manifold ───────────────────────────────────────────────────────────

// stubManifold answers ResolveArrow from a table keyed by the exact namespace
// it was asked for, and records every namespace so tests can assert on the
// requests made rather than only on what came back.
type stubManifold struct {
	mu        sync.Mutex
	requested []domain.Namespace
	resolve   func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, []byte, string, error)
}

func (s *stubManifold) ResolveArrow(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	s.mu.Lock()
	s.requested = append(s.requested, ns)
	s.mu.Unlock()
	return s.resolve(ctx, ns)
}

func (s *stubManifold) requests() []domain.Namespace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Namespace(nil), s.requested...)
}

func (s *stubManifold) ResolveCollection(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Collection, error) {
	return nil, errors.New("not used")
}

func (s *stubManifold) ParseCollection(
	_ []byte,
	_ domain.Namespace,
) (*domain.Collection, error) {
	return nil, errors.New("not used")
}

func (s *stubManifold) ParseArrow(
	_ []byte,
) (*domain.Arrow, error) {
	return nil, errors.New("not used")
}

func (s *stubManifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return "", errors.New("not used")
}

func (s *stubManifold) ResolveLatestStable(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errors.New("not used")
}

func (s *stubManifold) ResolveDefaultBranch(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errors.New("not used")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func newVault(t *testing.T) vault.Vault {
	t.Helper()
	root := t.TempDir()
	v, err := vault.New(filepath.Join(root, "vault"), filepath.Join(root, "namespaces"), time.Hour)
	require.NoError(t, err)
	return v
}

func candidate(ns, branch string) provider.Candidate {
	return provider.Candidate{
		Namespace:     domain.Namespace(ns),
		Name:          "chromium",
		Description:   "a browser",
		Stars:         42,
		Source:        domain.Namespace(ns).Domain(),
		DefaultBranch: branch,
	}
}

func resolvesTo(name string) func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
	return func(_ context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
		return &domain.Arrow{
			ArrowMeta: domain.ArrowMeta{
				Name:        name,
				Description: "a browser",
				Version:     "v1.0.0",
				Tags:        []string{"browser"},
			},
			Targets: map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
		}, []byte("schema: arrow@v0\n"), "ARROW.md", nil
	}
}

func neverKnown(
	_ context.Context,
	_ domain.Namespace,
) (bool, error) {
	return false, nil
}

func newDiscovery(
	t *testing.T,
	providers []provider.Provider,
	m *stubManifold,
	v vault.Vault,
	known discovery.KnownFn,
	mutate func(*discovery.Config),
) discovery.Discovery {
	t.Helper()

	cfg := discovery.Config{
		Topics:           []string{"quiver-arrow"},
		PerProviderLimit: 25,
		FetchConcurrency: 8,
	}
	if mutate != nil {
		mutate(&cfg)
	}

	d, err := discovery.New(providers, m, v, known, cfg)
	require.NoError(t, err)
	return d
}

type collector struct {
	mu      sync.Mutex
	results []discovery.Result
}

func (c *collector) emit(r discovery.Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = append(c.results, r)
}

func (c *collector) all() []discovery.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]discovery.Result(nil), c.results...)
}

// ─── construction ────────────────────────────────────────────────────────────

func TestNew_NilManifold_ReturnsError(t *testing.T) {
	_, err := discovery.New(nil, nil, newVault(t), neverKnown, discovery.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifold")
}

func TestNew_NilVault_ReturnsError(t *testing.T) {
	_, err := discovery.New(nil, &stubManifold{}, nil, neverKnown, discovery.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault")
}

// ─── happy path ──────────────────────────────────────────────────────────────

func TestDiscover_ValidManifestWritesVaultAndEmits(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "master"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Found)
	assert.Equal(t, 1, outcome.Verified)
	assert.Zero(t, outcome.Skipped)
	require.Len(t, outcome.Providers, 1)
	assert.True(t, outcome.Providers[0].OK)
	assert.Equal(t, 1, outcome.Providers[0].Returned)

	results := got.all()
	require.Len(t, results, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/chromium"), results[0].Namespace)
	assert.Equal(t, "Chromium", results[0].Arrow.Name)
	// The branch the manifest came from wins over whatever version the manifest
	// itself states, which is exactly the disagreement this design removes.
	assert.Equal(t, "master", results[0].Arrow.Version)
	assert.Equal(t, domain.Namespace("github.com/acme/chromium@master"), results[0].Arrow.Namespace)
	assert.Equal(t, 42, results[0].Stars)
	assert.Equal(t, "github.com", results[0].Source)
	assert.False(t, results[0].Known)

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "Chromium", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/chromium"), rows[0].Namespace)
	assert.Equal(t, "master", rows[0].Ref)
	assert.Equal(t, 42, rows[0].Meta.Stars)
	assert.Equal(t, "github.com", rows[0].Meta.Source)
	assert.Equal(t, "master", rows[0].Meta.Branch)
	assert.Equal(t, []domain.OS{domain.OSLinuxAMD64}, rows[0].Meta.OS)
}

func TestDiscover_UnparseableManifestIsSkippedNotEmitted(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/junk", "main"),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", errors.New("manifest is not valid yaml")
		},
	}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, nil).
		Discover(context.Background(), "junk", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Found)
	assert.Zero(t, outcome.Verified)
	assert.Equal(t, 1, outcome.Skipped)
	assert.Empty(t, got.all())

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "junk", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestDiscover_DuplicateAcrossProvidersEmitsOnce(t *testing.T) {
	shared := candidate("github.com/acme/chromium", "main")
	first := &stubProvider{host: "github.com", candidates: []provider.Candidate{shared}}
	second := &stubProvider{host: "gitlab.com", candidates: []provider.Candidate{shared}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{first, second}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Found)
	assert.Equal(t, 1, outcome.Verified)
	assert.Len(t, got.all(), 1)
	assert.Len(t, m.requests(), 1)
}

// A candidate that differs only by ref is still one repository.
func TestDiscover_DedupesOnTheBareNamespace(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
		candidate("github.com/acme/chromium@v2", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Found)
	assert.Len(t, got.all(), 1)
}

// ─── known candidates ────────────────────────────────────────────────────────

func TestDiscover_KnownCandidateIsFlaggedNotDropped(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	known := func(_ context.Context, _ domain.Namespace) (bool, error) {
		return true, nil
	}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), known, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Verified)
	results := got.all()
	require.Len(t, results, 1)
	assert.True(t, results[0].Known)
}

// The check is against the stores, so a candidate the vault already holds is
// known even when the catalog has never heard of it.
func TestDiscover_CandidateAlreadyInTheVaultIsFlaggedKnown(t *testing.T) {
	v := newVault(t)
	require.NoError(t, v.PutArrow(
		context.Background(),
		domain.Namespace("github.com/acme/chromium@main"),
		vault.ManifestFile{Content: []byte("cached"), Filename: "ARROW.md"},
	))

	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	_, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	results := got.all()
	require.Len(t, results, 1)
	assert.True(t, results[0].Known)
}

// Losing the catalog lookup must not lose the result, only the flag.
func TestDiscover_KnownLookupError_StillEmitsUnflagged(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	known := func(_ context.Context, _ domain.Namespace) (bool, error) {
		return false, errors.New("catalog unavailable")
	}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), known, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Verified)
	results := got.all()
	require.Len(t, results, 1)
	assert.False(t, results[0].Known)
}

func TestDiscover_NilKnownFn_TreatsEverythingAsNew(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), nil, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	results := got.all()
	require.Len(t, results, 1)
	assert.False(t, results[0].Known)
}

// ─── provider failure ────────────────────────────────────────────────────────

func TestDiscover_OneProviderRateLimitedOtherSucceeds(t *testing.T) {
	limited := &stubProvider{
		host: "github.com",
		err:  &provider.RateLimitedError{Host: "github.com", RetryAfter: 40 * time.Second},
	}
	working := &stubProvider{host: "gitlab.com", candidates: []provider.Candidate{
		candidate("gitlab.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{limited, working}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Len(t, got.all(), 1, "a rate-limited host must not stop the others")
	assert.Equal(t, 1, outcome.Verified)

	require.Len(t, outcome.Providers, 2)
	byHost := map[string]discovery.ProviderOutcome{}
	for _, po := range outcome.Providers {
		byHost[po.Host] = po
	}

	assert.False(t, byHost["github.com"].OK)
	assert.Equal(t, discovery.ReasonRateLimited, byHost["github.com"].Reason)
	assert.Equal(t, 40*time.Second, byHost["github.com"].RetryAfter)

	assert.True(t, byHost["gitlab.com"].OK)
	assert.Equal(t, 1, byHost["gitlab.com"].Returned)
}

func TestDiscover_UnauthorizedProviderIsReportedAsSuch(t *testing.T) {
	p := &stubProvider{host: "github.com", err: &provider.UnauthorizedError{Host: "github.com"}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, outcome.Providers, 1)
	assert.Equal(t, discovery.ReasonUnauthorized, outcome.Providers[0].Reason)
	assert.Zero(t, outcome.Providers[0].RetryAfter)
}

func TestDiscover_AllProvidersFailReturnsOutcomeNotError(t *testing.T) {
	first := &stubProvider{host: "github.com", err: &provider.RateLimitedError{Host: "github.com"}}
	second := &stubProvider{host: "gitlab.com", err: errors.New("dns failure")}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{first, second}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err, "every provider failing is a reportable outcome, not a broken pipeline")

	assert.Zero(t, outcome.Found)
	assert.Zero(t, outcome.Verified)
	assert.Empty(t, got.all())
	require.Len(t, outcome.Providers, 2)
	for _, po := range outcome.Providers {
		assert.False(t, po.OK)
	}
	assert.Equal(t, discovery.ReasonRateLimited, outcome.Providers[0].Reason)
	assert.Equal(t, discovery.ReasonError, outcome.Providers[1].Reason)
}

func TestDiscover_NoProviders_ReturnsEmptyOutcome(t *testing.T) {
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, nil, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)
	assert.Zero(t, outcome.Found)
	assert.Empty(t, outcome.Providers)
}

func TestDiscover_EmptyText_ReturnsError(t *testing.T) {
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, nil, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "   ", func(discovery.Result) {})
	require.Error(t, err)
}

// ─── query shape ─────────────────────────────────────────────────────────────

func TestDiscover_SendsTopicsAndLimitToEveryProvider(t *testing.T) {
	queries := make(chan provider.SearchRequest, 1)
	p := &stubProvider{host: "github.com", queries: queries}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, func(c *discovery.Config) {
		c.Topics = []string{"quiver-arrow", "quiver-beta"}
		c.PerProviderLimit = 3
	}).Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	req := <-queries
	assert.Equal(t, "browser", req.Text)
	assert.Equal(t, []string{"quiver-arrow", "quiver-beta"}, req.Topics)
	assert.Equal(t, 3, req.Limit)
}

// The branch comes off the search response, so the manifest fetch must address
// it directly instead of walking the default-branch list.
func TestDiscover_BranchFromCandidateIsUsedForFetch(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "master"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(
		t,
		[]domain.Namespace{"github.com/acme/chromium@master"},
		m.requests(),
	)
}

func TestDiscover_CandidateWithoutABranch_ResolvesReflessly(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", ""),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, []domain.Namespace{"github.com/acme/chromium"}, m.requests())
}

// A candidate whose host named no default branch has no ref to index under, so
// the bare namespace is the key rather than an empty-ref row.
func TestDiscover_CandidateWithoutDefaultBranch_IndexesTheBareNamespace(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", ""),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return &domain.Arrow{
				ArrowMeta: domain.ArrowMeta{Name: "Chromium"},
			}, []byte("schema: arrow@v0\n"), "ARROW.md", nil
		},
	}

	_, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "Chromium", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0].Ref)
}

// ─── vault failure ───────────────────────────────────────────────────────────

func TestDiscover_VaultWriteFailure_CountsAsSkipped(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, &failingVault{}, neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Zero(t, outcome.Verified)
	assert.Equal(t, 1, outcome.Skipped)
	assert.Empty(t, got.all())
}

// failingVault refuses every write; every read says nothing is cached.
type failingVault struct {
	vault.Vault
}

func (f *failingVault) GetArrow(
	_ context.Context,
	_ domain.Namespace,
) (vault.ManifestFile, error) {
	return vault.ManifestFile{}, vault.ErrNotCached
}

func (f *failingVault) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	_ vault.ManifestFile,
) error {
	return errors.New("disk full")
}

// ─── concurrency ─────────────────────────────────────────────────────────────

func TestDiscover_ConcurrencyBoundRespected(t *testing.T) {
	const bound = 2
	const candidates = 8

	cands := make([]provider.Candidate, 0, candidates)
	for i := range candidates {
		cands = append(cands, candidate(fmt.Sprintf("github.com/acme/pkg%d", i), "main"))
	}

	var inFlight, peak atomic.Int64
	entered := make(chan struct{}, candidates)
	release := make(chan struct{})

	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			current := inFlight.Add(1)
			for {
				best := peak.Load()
				if current <= best || peak.CompareAndSwap(best, current) {
					break
				}
			}
			entered <- struct{}{}
			<-release
			inFlight.Add(-1)
			return resolvesTo("Chromium")(context.Background(), "")
		},
	}

	p := &stubProvider{host: "github.com", candidates: cands}
	d := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, func(c *discovery.Config) {
		c.FetchConcurrency = bound
	})

	done := make(chan discovery.Outcome, 1)
	go func() {
		outcome, err := d.Discover(context.Background(), "browser", func(discovery.Result) {})
		assert.NoError(t, err)
		done <- outcome
	}()

	// Once `bound` fetches are parked inside the stub, a correct pool cannot
	// start another until one of them returns.
	for range bound {
		<-entered
	}
	select {
	case <-entered:
		t.Fatal("more than the configured fetches ran concurrently")
	default:
	}

	close(release)
	outcome := <-done

	assert.Equal(t, int64(bound), peak.Load())
	assert.Equal(t, candidates, outcome.Verified)
}

func TestDiscover_ZeroConcurrency_StillMakesProgress(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, func(c *discovery.Config) {
		c.FetchConcurrency = 0
	}).Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)
	assert.Equal(t, 1, outcome.Verified)
}

// ─── streaming ───────────────────────────────────────────────────────────────

// A verified arrow must reach the caller while its neighbours are still being
// fetched, not once the whole pass is done.
func TestDiscover_EmitIsCalledAsEachVerifies(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	m := &stubManifold{
		resolve: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, []byte, string, error) {
			if ns.BareNamespace() == "github.com/acme/slow" {
				<-release
			}
			return resolvesTo("Chromium")(context.Background(), ns)
		},
	}

	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/slow", "main"),
		candidate("github.com/acme/fast", "main"),
	}}

	emitted := make(chan discovery.Result, 2)
	d := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil)

	done := make(chan struct{})
	go func() {
		_, err := d.Discover(context.Background(), "browser", func(r discovery.Result) { emitted <- r })
		assert.NoError(t, err)
		close(done)
	}()

	// This read completes only if the fast arrow is emitted while the slow one
	// is still parked in the stub.
	first := <-emitted
	assert.Equal(t, domain.Namespace("github.com/acme/fast"), first.Namespace)

	close(release)
	<-done

	second := <-emitted
	assert.Equal(t, domain.Namespace("github.com/acme/slow"), second.Namespace)
}

func TestDiscover_NilEmit_DoesNotPanic(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, outcome.Verified)
}

// ─── cancellation ────────────────────────────────────────────────────────────

func TestDiscover_ContextCancellationStopsPromptly(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	entered := make(chan struct{}, 8)
	m := &stubManifold{
		resolve: func(ctx context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
			entered <- struct{}{}
			select {
			case <-ctx.Done():
				return nil, nil, "", ctx.Err()
			case <-release:
				return resolvesTo("Chromium")(context.Background(), "")
			}
		},
	}

	cands := make([]provider.Candidate, 0, 8)
	for i := range 8 {
		cands = append(cands, candidate(fmt.Sprintf("github.com/acme/pkg%d", i), "main"))
	}
	p := &stubProvider{host: "github.com", candidates: cands}

	d := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, func(c *discovery.Config) {
		c.FetchConcurrency = 2
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan discovery.Outcome, 1)
	go func() {
		outcome, err := d.Discover(ctx, "browser", func(discovery.Result) {})
		assert.NoError(t, err)
		done <- outcome
	}()

	<-entered
	cancel()

	// Discover returns because the workers observe the cancelled context, not
	// because the stub was released.
	outcome := <-done
	assert.Zero(t, outcome.Verified)
}

func TestDiscover_CancelledBeforeStart_ReturnsWithoutFetching(t *testing.T) {
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(ctx, "browser", func(discovery.Result) {})
	require.NoError(t, err)
	assert.Zero(t, outcome.Verified)
	assert.Empty(t, m.requests())
}
