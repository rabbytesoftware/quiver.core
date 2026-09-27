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

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// ─── stub provider ───────────────────────────────────────────────────────────

type stubProvider struct {
	host               string
	candidates         []provider.Candidate
	err                error
	unmarkedCandidates []provider.Candidate
	unmarkedErr        error
	queries            chan provider.SearchRequest
	noSearch           bool
}

func (s *stubProvider) Host() string { return s.host }

func (s *stubProvider) CanSearch() bool { return !s.noSearch }

func (s *stubProvider) Search(
	_ context.Context,
	req provider.SearchRequest,
) ([]provider.Candidate, error) {
	if s.noSearch {
		return nil, provider.ErrSearchUnsupported
	}
	if s.queries != nil {
		s.queries <- req
	}
	if req.Unmarked {
		return s.unmarkedCandidates, s.unmarkedErr
	}
	return s.candidates, s.err
}

// The host questions below are the rest of the provider contract. Discovery
// asks a provider to search and nothing else, so a stub that is asked one of
// these has been wired somewhere it does not belong.
func (s *stubProvider) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errNotDiscovery
}

func (s *stubProvider) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", errNotDiscovery
}

func (s *stubProvider) DefaultBranches() []string { return nil }

var errNotDiscovery = errors.New("stub provider: discovery never asks this")

// ─── stub manifold ───────────────────────────────────────────────────────────

type stubManifold struct {
	mu        sync.Mutex
	requested []domain.Namespace
	probed    []domain.Namespace
	probeHint []fletcher.Hint
	resolve   func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, []byte, string, error)
	probe     func(ctx context.Context, ns domain.Namespace, hint fletcher.Hint) (*domain.Arrow, []byte, error)
}

func (s *stubManifold) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	return nil, nil, "", errors.New("not used")
}

func (s *stubManifold) ResolveDeclaredArrow(
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

func (s *stubManifold) ResolveArrowAt(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (*domain.Arrow, []byte, string, error) {
	return nil, nil, "", errors.New("not used")
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
) (string, string, error) {
	return "", "", errors.New("not used")
}

func (s *stubManifold) ResolveLatestInChannel(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return "", errors.New("not used")
}

func (s *stubManifold) ListChannels(
	_ context.Context,
	_ domain.Namespace,
) ([]manifold.ChannelInfo, error) {
	return nil, errors.New("not used")
}

func (s *stubManifold) ProbeArrow(
	ctx context.Context,
	ns domain.Namespace,
	hint fletcher.Hint,
) (*domain.Arrow, []byte, error) {
	s.mu.Lock()
	s.probed = append(s.probed, ns)
	s.probeHint = append(s.probeHint, hint)
	s.mu.Unlock()
	if s.probe != nil {
		return s.probe(ctx, ns, hint)
	}
	return nil, nil, errors.New("not used")
}

func (s *stubManifold) probes() []domain.Namespace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Namespace(nil), s.probed...)
}

func (s *stubManifold) hints() []fletcher.Hint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fletcher.Hint(nil), s.probeHint...)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func newVault(t *testing.T) vault.Vault {
	t.Helper()
	root := t.TempDir()
	v, err := vault.New(filepath.Join(root, "vault"), filepath.Join(root, "namespaces"), time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
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
	// The branch the manifest came from is the revision, and the namespace is the
	// only place it is recorded — which is exactly the disagreement this removes.
	assert.Equal(t, domain.Namespace("github.com/acme/chromium@master"), results[0].Arrow.Namespace)
	assert.Equal(t, 42, results[0].Stars)
	assert.Equal(t, "github.com", results[0].Source)
	assert.False(t, results[0].Known())

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
	assert.True(t, results[0].InCatalog)
	assert.False(t, results[0].InVault, "the vault is empty; only the catalog answered")
	assert.True(t, results[0].Known())
}

// The check is against the stores, so a candidate the vault already holds is
// known even when the catalog has never heard of it. Cached is not installed:
// the catalog flag must stay false, because it is what the client uses to tell
// what the user has from what they could have.
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
	assert.True(t, results[0].InVault)
	assert.False(t, results[0].InCatalog, "a cached manifest was never installed")
	assert.True(t, results[0].Known())
}

// Discovery indexes every arrow it proves, so a pass writes the very rows the
// next pass reads. An arrow seen on an earlier pass and never installed has to
// come back known but not in the catalog: reporting it installed would make the
// re-query claim the user has an arrow they only browsed.
func TestDiscover_SecondPassOverTheSameVault_IsKnownButNotInstalled(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}
	d := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, nil)

	var first collector
	_, err := d.Discover(context.Background(), "browser", first.emit)
	require.NoError(t, err)
	require.Len(t, first.all(), 1)
	require.False(t, first.all()[0].Known(), "nothing held this arrow before the first pass")

	var second collector
	_, err = d.Discover(context.Background(), "browser", second.emit)
	require.NoError(t, err)

	results := second.all()
	require.Len(t, results, 1)
	assert.True(t, results[0].InVault, "the first pass indexed it")
	assert.False(t, results[0].InCatalog, "indexing an arrow does not install it")
	assert.True(t, results[0].Known())
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
	assert.False(t, results[0].Known())
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
	assert.False(t, results[0].Known())
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

// Every platform is a provider so its manifests stay fetchable, but a host with
// no search API — bitbucket.org — has nothing to contribute to a query. It is
// never asked and never reported: an outcome would read as a host that failed
// rather than one that was never asked.
func TestDiscover_ProviderThatCannotSearchIsSkipped(t *testing.T) {
	fetchOnly := &stubProvider{host: "bitbucket.org", noSearch: true}
	searchable := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "master"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{fetchOnly, searchable}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	require.Len(t, outcome.Providers, 1, "only the hosts that were asked are reported")
	assert.Equal(t, "github.com", outcome.Providers[0].Host)
	assert.True(t, outcome.Providers[0].OK)
	assert.Len(t, got.all(), 1)
}

// A pass with nothing searchable at all is an empty pass, not a failure.
func TestDiscover_NoSearchableProviders_FindsNothing(t *testing.T) {
	fetchOnly := &stubProvider{host: "bitbucket.org", noSearch: true}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, []provider.Provider{fetchOnly}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	assert.Zero(t, outcome.Found)
	assert.Empty(t, outcome.Providers)
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

func withFletcher(
	cfg discovery.FletcherConfig,
) func(*discovery.Config) {
	return func(c *discovery.Config) {
		c.Fletcher = cfg
	}
}

func TestDiscover_FletcherDisabled_NoUnmarkedSearchIssued(t *testing.T) {
	queries := make(chan provider.SearchRequest, 2)
	p := &stubProvider{host: "github.com", queries: queries, candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, queries, 1, "fletcher disabled must issue only the tagged search")
	assert.False(t, (<-queries).Unmarked)
}

func TestDiscover_FletcherEnabled_SecondSearchIsUnmarkedWithMinStarsAndNoTopics(t *testing.T) {
	queries := make(chan provider.SearchRequest, 2)
	p := &stubProvider{host: "github.com", queries: queries, candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 75, ProbeLimit: 5}),
	).Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, queries, 2)
	tagged := <-queries
	assert.False(t, tagged.Unmarked)

	unmarked := <-queries
	assert.True(t, unmarked.Unmarked)
	assert.Equal(t, 75, unmarked.MinStars)
	assert.Empty(t, unmarked.Topics)
	assert.Equal(t, 10, unmarked.Limit, "limit is 2x ProbeLimit, to leave room for dedupe")
}

func TestDiscover_FletcherEnabled_TaggedDuplicateIsNotResolvedTwice(t *testing.T) {
	shared := candidate("github.com/acme/chromium", "main")
	p := &stubProvider{
		host:               "github.com",
		candidates:         []provider.Candidate{shared},
		unmarkedCandidates: []provider.Candidate{shared},
	}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Found)
	assert.Equal(t, 1, outcome.Verified)
	assert.Len(t, got.all(), 1)
	assert.Len(t, m.requests(), 1, "a candidate the tagged pass already proved is never re-resolved")
}

func TestDiscover_FletcherEnabled_UnmarkedResultsAreCappedAtProbeLimit(t *testing.T) {
	cands := make([]provider.Candidate, 0, 5)
	for i := range 5 {
		cands = append(cands, candidate(fmt.Sprintf("github.com/acme/probe%d", i), "main"))
	}
	p := &stubProvider{
		host:               "github.com",
		candidates:         []provider.Candidate{candidate("github.com/acme/tagged", "main")},
		unmarkedCandidates: cands,
	}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 2}),
	).Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, 3, outcome.Found, "1 tagged + 2 capped from the untagged pass")
	assert.Equal(t, 3, outcome.Verified)
	assert.Len(t, m.requests(), 3)
}

func TestDiscover_FletcherEnabled_TaggedCandidateWithoutManifestIsSkippedNeverIndexed(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/undeclared-tagged", "main"),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", fmt.Errorf("fetch: %w", manifoldresolver.ErrManifestNotFound)
		},
	}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "undeclared", got.emit)
	require.NoError(t, err)

	assert.Zero(t, outcome.Verified)
	assert.Equal(t, 1, outcome.Skipped)
	assert.Empty(t, got.all())
	assert.Empty(t, m.probes(), "the tagged pass never falls through to a probe")

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "undeclared", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows, "a tagged candidate with no manifest must never be fletched or indexed")
}

func TestDiscover_FletcherEnabled_DeclaredCandidateWithoutTopicUsesResolveArrowResult(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/undeclared", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Undeclared")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "undeclared", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Verified)
	results := got.all()
	require.Len(t, results, 1)
	assert.Equal(t, "Undeclared", results[0].Arrow.Name)
	assert.Empty(t, m.probes(), "a declared manifest is used as-is, never probed")

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "Undeclared", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1, "a declared arrow found without a topic is still indexed")
}

func TestDiscover_FletcherEnabled_NoManifestCandidateUsesProbeArrowResultAndIsNotIndexed(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/inferred", "main"),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", fmt.Errorf("fetch: %w", manifoldresolver.ErrManifestNotFound)
		},
		probe: func(context.Context, domain.Namespace, fletcher.Hint) (*domain.Arrow, []byte, error) {
			return &domain.Arrow{
				ArrowMeta: domain.ArrowMeta{
					Name:      "Inferred",
					Generator: &domain.ArrowGenerator{Name: "fletcher/1", Confidence: "low"},
				},
			}, []byte("schema: arrow@v0\n"), nil
		},
	}

	var got collector
	_, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "inferred", got.emit)
	require.NoError(t, err)

	results := got.all()
	require.Len(t, results, 1)
	assert.Equal(t, "Inferred", results[0].Arrow.Name)
	assert.Equal(t, "fletcher/1", results[0].Arrow.Generator.Name)

	require.Len(t, m.probes(), 1)
	assert.Equal(t, domain.Namespace("github.com/acme/inferred"), m.probes()[0])
	require.Len(t, m.hints(), 1)
	assert.Equal(t, "chromium", m.hints()[0].Name)
	assert.Equal(t, "a browser", m.hints()[0].Description)

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "Inferred", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows, "a probed arrow must never be written to the vault")
}

func TestDiscover_FletcherEnabled_ProbedResultKeepsTheProbedTagAsNamespaceRef(t *testing.T) {
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/inferred", "main"),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", fmt.Errorf("fetch: %w", manifoldresolver.ErrManifestNotFound)
		},
		probe: func(_ context.Context, ns domain.Namespace, _ fletcher.Hint) (*domain.Arrow, []byte, error) {
			return &domain.Arrow{
				Namespace: ns.WithRef("v2.0.0"),
				ArrowMeta: domain.ArrowMeta{
					Name:      "Inferred",
					Generator: &domain.ArrowGenerator{Name: "fletcher/1", Confidence: "low"},
				},
			}, []byte("schema: arrow@v0\n"), nil
		},
	}

	var got collector
	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "inferred", got.emit)
	require.NoError(t, err)

	results := got.all()
	require.Len(t, results, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/inferred@v2.0.0"), results[0].Arrow.Namespace)
	assert.Equal(t, "v2.0.0", results[0].Arrow.Namespace.Ref())

	rendered := apidto.SearchResultDTOFromDiscovery(results[0])
	assert.Contains(t, rendered.Versions, "v2.0.0")
}

func TestDiscover_FletcherEnabled_ProbedResultWithEmptyNamespaceFallsBackToBare(t *testing.T) {
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/inferred", "main"),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", fmt.Errorf("fetch: %w", manifoldresolver.ErrManifestNotFound)
		},
		probe: func(context.Context, domain.Namespace, fletcher.Hint) (*domain.Arrow, []byte, error) {
			return &domain.Arrow{
				ArrowMeta: domain.ArrowMeta{Name: "Inferred"},
			}, []byte("schema: arrow@v0\n"), nil
		},
	}

	var got collector
	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "inferred", got.emit)
	require.NoError(t, err)

	results := got.all()
	require.Len(t, results, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/inferred"), results[0].Arrow.Namespace)
}

func TestDiscover_FletcherEnabled_ProbeNotFletchableIsSkipped(t *testing.T) {
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/unfletchable", "main"),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", fmt.Errorf("fetch: %w", manifoldresolver.ErrManifestNotFound)
		},
		probe: func(context.Context, domain.Namespace, fletcher.Hint) (*domain.Arrow, []byte, error) {
			return nil, nil, fletcher.NotFletchableError{Reason: fletcher.ReasonNoReleaseAssets}
		},
	}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "unfletchable", got.emit)
	require.NoError(t, err)

	assert.Zero(t, outcome.Verified)
	assert.Equal(t, 1, outcome.Skipped)
	assert.Empty(t, got.all())
}

func TestDiscover_FletcherEnabled_TransientResolveErrorSkipsWithoutProbing(t *testing.T) {
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/transient", "main"),
	}}
	m := &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", errors.New("network unreachable")
		},
	}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "transient", got.emit)
	require.NoError(t, err)

	assert.Zero(t, outcome.Verified)
	assert.Equal(t, 1, outcome.Skipped)
	assert.Empty(t, got.all())
	assert.Empty(t, m.probes(), "a transient failure must never fall through to probe")
}

func TestDiscover_FletcherEnabled_ProviderUnsupportedForUnmarkedIsNotAFailure(t *testing.T) {
	tagged := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	gitlabLike := &stubProvider{host: "gitlab.com", unmarkedErr: provider.ErrSearchUnsupported}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, []provider.Provider{tagged, gitlabLike}, m, newVault(t), neverKnown,
		withFletcher(discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	var unmarkedOutcome *discovery.ProviderOutcome
	for i := range outcome.Providers {
		if outcome.Providers[i].Host == "gitlab.com" && outcome.Providers[i].Pass == discovery.PassUnmarked {
			unmarkedOutcome = &outcome.Providers[i]
		}
	}
	require.NotNil(t, unmarkedOutcome)
	assert.True(t, unmarkedOutcome.OK, "an unsupported unmarked search is not a failure")
	assert.Equal(t, discovery.ReasonUnsupported, unmarkedOutcome.Reason)
}

func TestDiscover_FletcherEnabled_UnmarkedPassRespectsTheSharedConcurrencyBound(t *testing.T) {
	const bound = 2
	const candidates = 6

	cands := make([]provider.Candidate, 0, candidates)
	for i := range candidates {
		cands = append(cands, candidate(fmt.Sprintf("github.com/acme/probe%d", i), "main"))
	}
	p := &stubProvider{host: "github.com", unmarkedCandidates: cands}

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

	d := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, func(c *discovery.Config) {
		c.FetchConcurrency = bound
		c.Fletcher = discovery.FletcherConfig{Enabled: true, MinStars: 10, ProbeLimit: candidates}
	})

	done := make(chan discovery.Outcome, 1)
	go func() {
		outcome, err := d.Discover(context.Background(), "browser", func(discovery.Result) {})
		assert.NoError(t, err)
		done <- outcome
	}()

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
