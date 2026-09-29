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

func (s *stubProvider) BlobFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (s *stubProvider) RepoPageURL(
	_ domain.Namespace,
) string {
	return ""
}

func (s *stubProvider) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]domain.ReleaseAsset, error) {
	return nil, nil
}

func (s *stubProvider) DefaultBranches() []string { return nil }

var errNotDiscovery = errors.New("stub provider: discovery never asks this")

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
		Unmarked:         discovery.UnmarkedConfig{MinStars: 10, ProbeLimit: 10},
	}
	if mutate != nil {
		mutate(&cfg)
	}

	d, err := discovery.New(providers, m, v, known, cfg)
	require.NoError(t, err)
	return d
}

func taggedProviders(outcome discovery.Outcome) []discovery.ProviderOutcome {
	var tagged []discovery.ProviderOutcome
	for _, po := range outcome.Providers {
		if po.Pass == discovery.PassTagged {
			tagged = append(tagged, po)
		}
	}
	return tagged
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
	d, err := discovery.New(nil, nil, newVault(t), neverKnown, discovery.Config{})
	require.Error(t, err)
	assert.Nil(t, d)
}

func TestNew_NilVault_ReturnsError(t *testing.T) {
	d, err := discovery.New(nil, &stubManifold{}, nil, neverKnown, discovery.Config{})
	require.Error(t, err)
	assert.Nil(t, d)
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
	tagged := taggedProviders(outcome)
	require.Len(t, tagged, 1)
	assert.True(t, tagged[0].OK)
	assert.Equal(t, 1, tagged[0].Returned)

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

	tagged := taggedProviders(outcome)
	require.Len(t, tagged, 2)
	byHost := map[string]discovery.ProviderOutcome{}
	for _, po := range tagged {
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

	tagged := taggedProviders(outcome)
	require.Len(t, tagged, 1, "only the hosts that were asked are reported")
	assert.Equal(t, "github.com", tagged[0].Host)
	assert.True(t, tagged[0].OK)
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

	tagged := taggedProviders(outcome)
	require.Len(t, tagged, 1)
	assert.Equal(t, discovery.ReasonUnauthorized, tagged[0].Reason)
	assert.Zero(t, tagged[0].RetryAfter)
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
	tagged := taggedProviders(outcome)
	require.Len(t, tagged, 2)
	for _, po := range tagged {
		assert.False(t, po.OK)
	}
	assert.Equal(t, discovery.ReasonRateLimited, tagged[0].Reason)
	assert.Equal(t, discovery.ReasonError, tagged[1].Reason)
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
	queries := make(chan provider.SearchRequest, 2)
	p := &stubProvider{host: "github.com", queries: queries}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, func(c *discovery.Config) {
		c.Topics = []string{"quiver-arrow", "quiver-beta"}
		c.PerProviderLimit = 3
		c.Unmarked = discovery.UnmarkedConfig{MinStars: 7, ProbeLimit: 4}
	}).Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, queries, 2)
	tagged := <-queries
	assert.Equal(t, "browser", tagged.Text)
	assert.False(t, tagged.Unmarked)
	assert.Equal(t, []string{"quiver-arrow", "quiver-beta"}, tagged.Topics)
	assert.Equal(t, 3, tagged.Limit)

	unmarked := <-queries
	assert.Equal(t, "browser", unmarked.Text)
	assert.True(t, unmarked.Unmarked)
	assert.Empty(t, unmarked.Topics)
	assert.Equal(t, 7, unmarked.MinStars)
	assert.Equal(t, 8, unmarked.Limit)
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

func withUnmarkedConfig(
	cfg discovery.UnmarkedConfig,
) func(*discovery.Config) {
	return func(c *discovery.Config) {
		c.Unmarked = cfg
	}
}

func TestDiscover_Unmarked_RunsWithoutAnyFlag(t *testing.T) {
	queries := make(chan provider.SearchRequest, 2)
	p := &stubProvider{host: "github.com", queries: queries, candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, nil).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, queries, 2, "the unmarked pass always follows the tagged one")
	assert.False(t, (<-queries).Unmarked)
	assert.True(t, (<-queries).Unmarked)
}

func TestDiscover_Unmarked_SecondSearchIsUnmarkedWithMinStarsAndNoTopics(t *testing.T) {
	queries := make(chan provider.SearchRequest, 2)
	p := &stubProvider{host: "github.com", queries: queries, candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withUnmarkedConfig(discovery.UnmarkedConfig{MinStars: 75, ProbeLimit: 5}),
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

func TestDiscover_Unmarked_TaggedDuplicateIsNotResolvedTwice(t *testing.T) {
	shared := candidate("github.com/acme/chromium", "main")
	p := &stubProvider{
		host:               "github.com",
		candidates:         []provider.Candidate{shared},
		unmarkedCandidates: []provider.Candidate{shared},
	}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	var got collector
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown,
		withUnmarkedConfig(discovery.UnmarkedConfig{MinStars: 10, ProbeLimit: 5}),
	).Discover(context.Background(), "browser", got.emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Found)
	assert.Equal(t, 1, outcome.Verified)
	assert.Len(t, got.all(), 1)
	assert.Len(t, m.requests(), 1, "a candidate the tagged pass already proved is never re-resolved")
}

func unmarkedManifold(
	arrow *domain.Arrow,
) *stubManifold {
	return &stubManifold{
		resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			copied := *arrow
			return &copied, []byte("schema: arrow@v0\n"), "ARROW.md", nil
		},
	}
}

func inferredArrow() *domain.Arrow {
	return &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:      "Inferred",
			Tags:      []string{"editor"},
			Generator: &domain.ArrowGenerator{Name: "generator/1", Confidence: "high"},
		},
		Targets: map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
	}
}

func withUnmarked(
	probeLimit int,
) func(*discovery.Config) {
	return withUnmarkedConfig(discovery.UnmarkedConfig{MinStars: 10, ProbeLimit: probeLimit})
}

func TestDiscover_Unmarked_UnmarkedResultsAreCappedAtProbeLimit(t *testing.T) {
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

	outcome, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, withUnmarked(2)).
		Discover(context.Background(), "browser", func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, 3, outcome.Found, "1 tagged + 2 capped from the untagged pass")
	assert.Equal(t, 3, outcome.Verified)
	assert.Len(t, m.requests(), 3)
}

func TestDiscover_Unmarked_UnmarkedCandidateIsCachedBeforeStreaming(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/inferred", "main"),
	}}
	m := unmarkedManifold(inferredArrow())

	var cachedWhenStreamed []error
	emit := func(r discovery.Result) {
		_, err := v.GetArrow(context.Background(), r.Arrow.Namespace)
		cachedWhenStreamed = append(cachedWhenStreamed, err)
	}
	outcome, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, withUnmarked(5)).
		Discover(context.Background(), "inferred", emit)
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Verified)
	require.Equal(t, []error{nil}, cachedWhenStreamed, "a result streams only once its manifest is cached")
	assert.Equal(t, []domain.Namespace{"github.com/acme/inferred@main"}, m.requests())

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "Inferred", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/inferred"), rows[0].Namespace)
	assert.Equal(t, "main", rows[0].Ref)
	assert.Equal(t, "main", rows[0].Meta.Branch)
	assert.Equal(t, 42, rows[0].Meta.Stars)
	assert.Equal(t, "github.com", rows[0].Meta.Source)
	assert.Equal(t, []domain.OS{domain.OSLinuxAMD64}, rows[0].Meta.OS)
	assert.Equal(t, []string{"editor"}, rows[0].Meta.Arrow.Tags)
	assert.Equal(t, "high", rows[0].Meta.Arrow.Generator.Confidence)
}

func TestDiscover_Unmarked_UnmarkedResultCarriesTheCandidateBranch(t *testing.T) {
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/inferred", "main"),
	}}
	m := unmarkedManifold(inferredArrow())

	var got collector
	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), neverKnown, withUnmarked(5)).
		Discover(context.Background(), "inferred", got.emit)
	require.NoError(t, err)

	results := got.all()
	require.Len(t, results, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/inferred"), results[0].Namespace)
	assert.Equal(t, domain.Namespace("github.com/acme/inferred@main"), results[0].Arrow.Namespace)
	assert.Equal(t, 42, results[0].Stars)
	assert.Equal(t, "github.com", results[0].Source)
	assert.False(t, results[0].InVault, "nothing held the arrow before the pass cached it")

	rendered := apidto.SearchResultDTOFromDiscovery(results[0])
	assert.Contains(t, rendered.Versions, "main")
}

func TestDiscover_Unmarked_UnmarkedCandidateAlreadyInTheVaultIsFlaggedKnown(t *testing.T) {
	testCases := []struct {
		name      string
		cachedRef domain.Namespace
		wantKnown bool
	}{
		{name: "same ref", cachedRef: "github.com/acme/inferred@main", wantKnown: true},
		{name: "other ref", cachedRef: "github.com/acme/inferred@v1.0.0", wantKnown: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := newVault(t)
			require.NoError(t, v.PutArrow(context.Background(), tc.cachedRef, vault.ManifestFile{
				Content: []byte("cached"), Filename: "ARROW.md",
			}))
			p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
				candidate("github.com/acme/inferred", "main"),
			}}
			m := unmarkedManifold(inferredArrow())

			var got collector
			_, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, withUnmarked(5)).
				Discover(context.Background(), "inferred", got.emit)
			require.NoError(t, err)

			results := got.all()
			require.Len(t, results, 1)
			assert.Equal(t, tc.wantKnown, results[0].InVault)
			assert.False(t, results[0].InCatalog)
			assert.Equal(t, []domain.Namespace{"github.com/acme/inferred@main"}, m.requests())
		})
	}
}

func TestDiscover_Unmarked_UnmarkedKnownCandidateIsFlagged(t *testing.T) {
	p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
		candidate("github.com/acme/inferred", "main"),
	}}
	m := unmarkedManifold(inferredArrow())
	var asked []domain.Namespace
	known := func(_ context.Context, ns domain.Namespace) (bool, error) {
		asked = append(asked, ns)
		return true, nil
	}

	var got collector
	_, err := newDiscovery(t, []provider.Provider{p}, m, newVault(t), known, withUnmarked(5)).
		Discover(context.Background(), "inferred", got.emit)
	require.NoError(t, err)

	results := got.all()
	require.Len(t, results, 1)
	assert.True(t, results[0].InCatalog)
	assert.Equal(t, []domain.Namespace{"github.com/acme/inferred"}, asked)
}

func TestDiscover_Unmarked_UnmarkedFailuresAreSkippedNotEmitted(t *testing.T) {
	testCases := []struct {
		name    string
		vault   func(t *testing.T) vault.Vault
		resolve func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error)
	}{
		{
			name:  "cache write fails",
			vault: func(*testing.T) vault.Vault { return &failingVault{} },
			resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
				return inferredArrow(), []byte("schema: arrow@v0\n"), "ARROW.md", nil
			},
		},
		{
			name:  "manifold refuses the candidate",
			vault: newVault,
			resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
				return nil, nil, "", errors.New("no usable asset")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.vault(t)
			p := &stubProvider{host: "github.com", unmarkedCandidates: []provider.Candidate{
				candidate("github.com/acme/unresolvable", "main"),
			}}
			m := &stubManifold{resolve: tc.resolve}

			var got collector
			outcome, err := newDiscovery(t, []provider.Provider{p}, m, v, neverKnown, withUnmarked(5)).
				Discover(context.Background(), "unresolvable", got.emit)
			require.NoError(t, err)

			assert.Zero(t, outcome.Verified)
			assert.Equal(t, 1, outcome.Skipped)
			assert.Empty(t, got.all())
			assert.NotEmpty(t, m.requests())
		})
	}
}

func TestDiscover_Unmarked_ProviderUnsupportedForUnmarkedIsNotAFailure(t *testing.T) {
	tagged := &stubProvider{host: "github.com", candidates: []provider.Candidate{
		candidate("github.com/acme/chromium", "main"),
	}}
	gitlabLike := &stubProvider{host: "gitlab.com", unmarkedErr: provider.ErrSearchUnsupported}
	m := &stubManifold{resolve: resolvesTo("Chromium")}

	outcome, err := newDiscovery(t, []provider.Provider{tagged, gitlabLike}, m, newVault(t), neverKnown, withUnmarked(5)).
		Discover(context.Background(), "browser", func(discovery.Result) {})
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

func TestDiscover_Unmarked_UnmarkedPassRespectsTheSharedConcurrencyBound(t *testing.T) {
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
		c.Unmarked = discovery.UnmarkedConfig{MinStars: 10, ProbeLimit: candidates}
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
