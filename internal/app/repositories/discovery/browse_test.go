package discovery_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

func browseSource(host string) discovery.BrowseSource {
	return discovery.BrowseSource{Host: host, Sort: "stars", MinStars: 500, PushedWithin: 24 * time.Hour, Limit: 10}
}

func withMaxStars(
	source discovery.BrowseSource,
	maxStars int,
) discovery.BrowseSource {
	source.MaxStars = maxStars
	return source
}

func oneAtATime(c *discovery.Config) {
	c.FetchConcurrency = 1
}

func absentFor(
	absent ...string,
) func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
	ok := resolvesTo("X")
	return func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, []byte, string, error) {
		for _, name := range absent {
			if ns.BareNamespace() == domain.Namespace("github.com/acme/"+name) {
				return notFound(ctx, ns)
			}
		}
		return ok(ctx, ns)
	}
}

func browseProvider(names ...string) *stubProvider {
	candidates := make([]provider.Candidate, 0, len(names))
	for _, name := range names {
		candidates = append(candidates, candidate("github.com/acme/"+name, "main"))
	}
	return &stubProvider{host: "github.com", unmarkedCandidates: candidates}
}

func parsesTo(name string) func([]byte) (*domain.Arrow, error) {
	return func([]byte) (*domain.Arrow, error) {
		return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: name}}, nil
	}
}

func notFound(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
	return nil, nil, "", fmt.Errorf("fletcher: %w", manifoldresolver.ErrManifestNotFound)
}

func TestBrowse_NoSources_ReturnsError(t *testing.T) {
	d := newDiscovery(t, nil, &stubManifold{}, newVault(t), neverKnown, nil)

	_, err := d.Browse(context.Background(), discovery.BrowseRequest{Budget: 1}, func(discovery.Result) {})

	require.Error(t, err)
}

func TestBrowse_SendsTheSourceToTheMatchingProviderOnly(t *testing.T) {
	queries := make(chan provider.SearchRequest, 4)
	github := browseProvider("a")
	github.queries = queries
	other := &stubProvider{host: "gitlab.com", queries: queries}
	d := newDiscovery(t, []provider.Provider{github, other}, &stubManifold{resolve: resolvesTo("A")}, newVault(t), neverKnown, nil)

	_, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{withMaxStars(browseSource("github"), 9000)},
		Budget:  5,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, queries, 1)
	assert.Equal(t, provider.SearchRequest{
		Unmarked:     true,
		Sort:         "stars",
		MinStars:     500,
		MaxStars:     9000,
		PushedWithin: 24 * time.Hour,
		Limit:        10,
	}, <-queries)
}

func TestBrowse_UnknownHost_IsReportedNotFatal(t *testing.T) {
	d := newDiscovery(t, []provider.Provider{browseProvider("a")}, &stubManifold{resolve: resolvesTo("A")}, newVault(t), neverKnown, nil)

	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("codeberg")},
		Budget:  5,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, outcome.Providers, 1)
	assert.Equal(t, "codeberg", outcome.Providers[0].Host)
	assert.Equal(t, discovery.PassBrowse, outcome.Providers[0].Pass)
	assert.Equal(t, discovery.ReasonUnsupported, outcome.Providers[0].Reason)
	assert.Zero(t, outcome.Found)
}

func TestBrowse_MergesSourcesInOrderAndDedupes(t *testing.T) {
	p := browseProvider("a", "b")
	d := newDiscovery(t, []provider.Provider{p}, &stubManifold{resolve: resolvesTo("X")}, newVault(t), neverKnown, nil)

	var got collector
	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github.com"), browseSource("github")},
		Budget:  5,
	}, got.emit)
	require.NoError(t, err)

	assert.Equal(t, 2, outcome.Found)
	assert.Equal(t, 2, outcome.Verified)
	assert.Equal(t, []domain.Namespace{"github.com/acme/a", "github.com/acme/b"}, outcome.Order)
	assert.Len(t, got.all(), 2)
	assert.Len(t, outcome.Providers, 2)
}

func TestBrowse_BudgetCapsNewCandidatesInSearchOrder(t *testing.T) {
	m := &stubManifold{resolve: resolvesTo("X")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a", "b", "c", "d")}, m, newVault(t), neverKnown, nil)

	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  2,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, 4, outcome.Found)
	assert.Equal(t, 2, outcome.Verified)
	assert.Equal(t, []domain.Namespace{"github.com/acme/a", "github.com/acme/b"}, outcome.Order)
	assert.ElementsMatch(t, []domain.Namespace{"github.com/acme/a@main", "github.com/acme/b@main"}, m.requests())
}

func TestBrowse_ZeroBudget_ResolvesNothingNew(t *testing.T) {
	m := &stubManifold{resolve: resolvesTo("X")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a")}, m, newVault(t), neverKnown, nil)

	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Empty(t, m.requests())
	assert.Empty(t, outcome.Order)
}

func TestBrowse_VaultFreshCandidate_BypassesBudgetAndHost(t *testing.T) {
	v := newVault(t)
	ctx := context.Background()
	require.NoError(t, v.PutArrow(ctx, "github.com/acme/a@v1.0.0", vault.ManifestFile{Content: []byte("x"), Filename: "ARROW.md"}))
	m := &stubManifold{resolve: resolvesTo("X"), parse: parsesTo("Cached A")}
	known := func(context.Context, domain.Namespace) (bool, error) { return true, nil }
	d := newDiscovery(t, []provider.Provider{browseProvider("a")}, m, v, known, nil)

	var got collector
	outcome, err := d.Browse(ctx, discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
	}, got.emit)
	require.NoError(t, err)

	assert.Empty(t, m.requests(), "a cached candidate never reaches the manifold")
	assert.Equal(t, 1, outcome.Verified)
	assert.Equal(t, []domain.Namespace{"github.com/acme/a"}, outcome.Order)
	results := got.all()
	require.Len(t, results, 1)
	assert.Equal(t, "Cached A", results[0].Arrow.Name)
	assert.Equal(t, domain.Namespace("github.com/acme/a@v1.0.0"), results[0].Arrow.Namespace)
	assert.Equal(t, domain.Namespace("github.com/acme/a"), results[0].Namespace)
	assert.Equal(t, 42, results[0].Stars)
	assert.True(t, results[0].InVault)
	assert.True(t, results[0].InCatalog)
}

func TestBrowse_UnparseableCachedManifest_IsResolvedAgain(t *testing.T) {
	v := newVault(t)
	ctx := context.Background()
	require.NoError(t, v.PutArrow(ctx, "github.com/acme/a@v1.0.0", vault.ManifestFile{Content: []byte("x"), Filename: "ARROW.md"}))
	m := &stubManifold{resolve: resolvesTo("X"), parse: func([]byte) (*domain.Arrow, error) { return nil, errors.New("bad") }}
	d := newDiscovery(t, []provider.Provider{browseProvider("a")}, m, v, neverKnown, nil)

	_, err := d.Browse(ctx, discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  1,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, []domain.Namespace{"github.com/acme/a@main"}, m.requests())
}

func TestBrowse_ConfirmedAbsentCandidate_IsDroppedWithoutAHostRequest(t *testing.T) {
	v := newVault(t)
	ctx := context.Background()
	require.NoError(t, v.PutArrowNotFound(ctx, "github.com/acme/a@main", ""))
	m := &stubManifold{resolve: resolvesTo("X")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a", "b")}, m, v, neverKnown, nil)

	outcome, err := d.Browse(ctx, discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, []domain.Namespace{"github.com/acme/b@main"}, m.requests())
	assert.Equal(t, 1, outcome.Skipped)
	assert.Equal(t, []domain.Namespace{"github.com/acme/b"}, outcome.Order)
}

func TestBrowse_NotFletchableOutcome_IsRecordedAsConfirmedAbsent(t *testing.T) {
	v := newVault(t)
	ctx := context.Background()
	m := &stubManifold{resolve: notFound}
	d := newDiscovery(t, []provider.Provider{browseProvider("a")}, m, v, neverKnown, nil)
	req := discovery.BrowseRequest{Sources: []discovery.BrowseSource{browseSource("github")}, Budget: 5}

	first, err := d.Browse(ctx, req, func(discovery.Result) {})
	require.NoError(t, err)
	second, err := d.Browse(ctx, req, func(discovery.Result) {})
	require.NoError(t, err)

	_, getErr := v.GetArrow(ctx, "github.com/acme/a@main")
	assert.ErrorIs(t, getErr, vault.ErrConfirmedAbsent)
	assert.Equal(t, 1, first.Skipped)
	assert.Equal(t, 1, second.Skipped)
	assert.Len(t, m.requests(), 1, "the second pass spends no request on a known miss")
}

func TestBrowse_TransientResolveFailure_IsNotRecordedAsAbsent(t *testing.T) {
	v := newVault(t)
	ctx := context.Background()
	m := &stubManifold{resolve: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
		return nil, nil, "", manifoldresolver.ErrFetchFailed
	}}
	d := newDiscovery(t, []provider.Provider{browseProvider("a")}, m, v, neverKnown, nil)

	_, err := d.Browse(ctx, discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	_, getErr := v.GetArrow(ctx, "github.com/acme/a@main")
	assert.ErrorIs(t, getErr, vault.ErrNotCached)
}

func TestBrowse_FailingMarkerWrite_DoesNotFailThePass(t *testing.T) {
	m := &stubManifold{resolve: notFound}
	d := newDiscovery(t, []provider.Provider{browseProvider("a")}, m, &markerRefusingVault{Vault: newVault(t)}, neverKnown, nil)

	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
	}, func(discovery.Result) {})

	require.NoError(t, err)
	assert.Equal(t, 1, outcome.Skipped)
}

type markerRefusingVault struct {
	vault.Vault
}

func (m *markerRefusingVault) PutArrowNotFound(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) error {
	return errors.New("disk full")
}

func TestBrowse_ProviderFailureIsReportedNotReturned(t *testing.T) {
	p := &stubProvider{host: "github.com", unmarkedErr: &provider.RateLimitedError{RetryAfter: time.Minute}}
	d := newDiscovery(t, []provider.Provider{p}, &stubManifold{resolve: resolvesTo("X")}, newVault(t), neverKnown, nil)

	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	require.Len(t, outcome.Providers, 1)
	assert.False(t, outcome.Providers[0].OK)
	assert.Equal(t, discovery.ReasonRateLimited, outcome.Providers[0].Reason)
	assert.Equal(t, discovery.PassBrowse, outcome.Providers[0].Pass)
}

func TestDiscover_NotFletchableOutcome_IsRecordedAsConfirmedAbsent(t *testing.T) {
	v := newVault(t)
	p := &stubProvider{host: "github.com", candidates: []provider.Candidate{candidate("github.com/acme/a", "main")}}
	d := newDiscovery(t, []provider.Provider{p}, &stubManifold{resolve: notFound}, v, neverKnown, nil)

	_, err := d.Discover(context.Background(), "a", func(discovery.Result) {})
	require.NoError(t, err)

	_, getErr := v.GetArrow(context.Background(), "github.com/acme/a@main")
	assert.ErrorIs(t, getErr, vault.ErrConfirmedAbsent)
}

func TestBrowse_WantStopsDispatchingOnceResultsReachIt(t *testing.T) {
	m := &stubManifold{resolve: absentFor("a")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a", "b", "c", "d", "e")}, m, newVault(t), neverKnown, oneAtATime)

	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
		Want:    2,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, 2, outcome.Verified)
	assert.Equal(t, 1, outcome.Skipped)
	assert.Equal(t, []domain.Namespace{"github.com/acme/a@main", "github.com/acme/b@main", "github.com/acme/c@main"}, m.requests())
}

func TestBrowse_WantKeepsLookingPastFailuresUntilBudget(t *testing.T) {
	m := &stubManifold{resolve: absentFor("a", "b", "c", "d")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a", "b", "c", "d", "e")}, m, newVault(t), neverKnown, oneAtATime)

	outcome, err := d.Browse(context.Background(), discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  3,
		Want:    2,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, 0, outcome.Verified)
	assert.Len(t, m.requests(), 3, "the budget stays the hard cap on new resolves")
}

func TestBrowse_WantCountsVaultFreshHits(t *testing.T) {
	v := newVault(t)
	ctx := context.Background()
	require.NoError(t, v.PutArrow(ctx, "github.com/acme/a@v1.0.0", vault.ManifestFile{Content: []byte("x"), Filename: "ARROW.md"}))
	m := &stubManifold{resolve: resolvesTo("X"), parse: parsesTo("Cached A")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a", "b", "c")}, m, v, neverKnown, oneAtATime)

	outcome, err := d.Browse(ctx, discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
		Want:    2,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, 2, outcome.Verified)
	assert.Equal(t, []domain.Namespace{"github.com/acme/b@main"}, m.requests())
}

func TestBrowse_WantMetByVaultAloneResolvesNothing(t *testing.T) {
	v := newVault(t)
	ctx := context.Background()
	require.NoError(t, v.PutArrow(ctx, "github.com/acme/a@v1.0.0", vault.ManifestFile{Content: []byte("x"), Filename: "ARROW.md"}))
	m := &stubManifold{resolve: resolvesTo("X"), parse: parsesTo("Cached A")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a", "b")}, m, v, neverKnown, oneAtATime)

	outcome, err := d.Browse(ctx, discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
		Want:    1,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Equal(t, 1, outcome.Verified)
	assert.Empty(t, m.requests())
}

func TestBrowse_WantWithCancelledContext_DispatchesNothing(t *testing.T) {
	m := &stubManifold{resolve: resolvesTo("X")}
	d := newDiscovery(t, []provider.Provider{browseProvider("a", "b")}, m, newVault(t), neverKnown, oneAtATime)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.Browse(ctx, discovery.BrowseRequest{
		Sources: []discovery.BrowseSource{browseSource("github")},
		Budget:  5,
		Want:    2,
	}, func(discovery.Result) {})
	require.NoError(t, err)

	assert.Empty(t, m.requests())
}

// A pass nobody is waiting on can ask for fewer resolves at once than the
// pipeline's own bound, so it leaves room for the requests someone is.
func TestBrowse_ConcurrencyNarrowsThePipelineBound(t *testing.T) {
	const narrow = 2
	names := []string{"a", "b", "c", "d", "e", "f"}

	var inFlight, peak atomic.Int64
	entered := make(chan struct{}, len(names))
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
			return resolvesTo("X")(context.Background(), "")
		},
	}
	d := newDiscovery(t, []provider.Provider{browseProvider(names...)}, m, newVault(t), neverKnown, func(c *discovery.Config) {
		c.FetchConcurrency = 8
	})

	done := make(chan error, 1)
	go func() {
		_, err := d.Browse(context.Background(), discovery.BrowseRequest{
			Sources:     []discovery.BrowseSource{browseSource("github")},
			Budget:      len(names),
			Concurrency: narrow,
		}, func(discovery.Result) {})
		done <- err
	}()

	for range narrow {
		<-entered
	}
	select {
	case <-entered:
		t.Fatal("more resolves ran at once than the pass asked for")
	default:
	}

	close(release)
	require.NoError(t, <-done)
	assert.Equal(t, int64(narrow), peak.Load())
}
