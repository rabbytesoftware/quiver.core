package manifold

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
)

func snapshotManifold(
	crs *stubConstraintResolver,
	clock *fakeClock,
) *manifold {
	return &manifold{
		trs:        translator.NewTranslator(),
		cmp:        compiler.New(),
		rls:        ruleset.New(),
		constraint: crs,
		hosts:      hostedBy(&stubHost{}),
		clock:      clock.Now,
		cacheTTL:   time.Hour,
	}
}

func TestSnapshot_SecondCallServedFromCache(t *testing.T) {
	snap := sharedSnapshot()
	crs := &stubConstraintResolver{refs: &snap}
	m := snapshotManifold(crs, &fakeClock{now: time.Now()})
	ns := domain.Namespace("github.com/u/r")

	first, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)
	second, err := m.Snapshot(context.Background(), ns.WithRef("stable"))
	require.NoError(t, err)

	assert.Equal(t, snap, first)
	assert.Equal(t, first, second)
	assert.Equal(t, 1, crs.refsCall)
}

func TestSnapshot_ExpiresAfterTTL(t *testing.T) {
	before := domain.RefSnapshot{Tags: map[string]string{"nightly": "old"}}
	crs := &stubConstraintResolver{refs: &before}
	now := time.Now()
	clock := &fakeClock{now: now}
	m := snapshotManifold(crs, clock)
	ns := domain.Namespace("github.com/u/r")

	_, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)

	after := domain.RefSnapshot{Tags: map[string]string{"nightly": "new"}}
	crs.refs = &after

	clock.now = now.Add(30 * time.Minute)
	cached, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, before, cached)

	clock.now = now.Add(time.Hour + time.Minute)
	fresh, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, after, fresh)
	assert.Equal(t, 2, crs.refsCall)
}

func TestSnapshot_ErrorIsNotCached(t *testing.T) {
	refsErr := errors.New("dial tcp: connection refused")
	crs := &stubConstraintResolver{refsErr: refsErr}
	m := snapshotManifold(crs, &fakeClock{now: time.Now()})
	ns := domain.Namespace("github.com/u/r")

	_, err := m.Snapshot(context.Background(), ns)
	assert.ErrorIs(t, err, refsErr)

	snap := sharedSnapshot()
	crs.refsErr = nil
	crs.refs = &snap
	got, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, snap, got)
	assert.Equal(t, 2, crs.refsCall)
}

// An update re-resolves right before it starts and again before it commits;
// a snapshot cached for the version-check TTL would hide a tag that moved in
// between, so FreshSnapshot always reads the remote and refreshes the cache.
func TestFreshSnapshot_BypassesAndRefreshesTheCache(t *testing.T) {
	before := domain.RefSnapshot{Tags: map[string]string{"nightly": "old"}}
	crs := &stubConstraintResolver{refs: &before}
	m := snapshotManifold(crs, &fakeClock{now: time.Now()})
	ns := domain.Namespace("github.com/u/r@nightly")

	_, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)

	after := domain.RefSnapshot{Tags: map[string]string{"nightly": "new"}}
	crs.refs = &after

	fresh, err := m.FreshSnapshot(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, after, fresh)

	cached, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, after, cached)
	assert.Equal(t, 2, crs.refsCall)
}

func TestFreshSnapshot_ErrorKeepsTheCachedSnapshot(t *testing.T) {
	snap := sharedSnapshot()
	crs := &stubConstraintResolver{refs: &snap}
	m := snapshotManifold(crs, &fakeClock{now: time.Now()})
	ns := domain.Namespace("github.com/u/r")

	_, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)

	refsErr := errors.New("dial tcp: connection refused")
	crs.refsErr = refsErr
	_, err = m.FreshSnapshot(context.Background(), ns)
	assert.ErrorIs(t, err, refsErr)

	crs.refsErr = nil
	cached, err := m.Snapshot(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, snap, cached)
	assert.Equal(t, 2, crs.refsCall)
}

func TestChannelsOf(t *testing.T) {
	testCases := []struct {
		name string
		snap domain.RefSnapshot
		want []ChannelInfo
	}{
		{
			name: "ordered channels, then pointer tags, never the branch once tags exist",
			snap: sharedSnapshot(),
			want: []ChannelInfo{
				{Name: "stable", Kind: "ordered", Latest: "stable-26.5.1", Count: 3, Members: []string{"stable-26.5.1", "v1.3.0", "v1.2.0"}},
				{Name: "beta", Kind: "ordered", Latest: "beta-26.5-4", Count: 2, Members: []string{"beta-26.5-4", "beta-26.5-1"}},
				{Name: "latest", Kind: "ordered", Latest: "v1.0-latest", Count: 1, Members: []string{"v1.0-latest"}},
				{Name: "nightly", Kind: "pointer", Latest: "nightly"},
				{Name: "nightly-latest", Kind: "pointer", Latest: "nightly-latest"},
			},
		},
		{
			name: "head branch fallback when there are no tags",
			snap: domain.RefSnapshot{Branches: map[string]string{"develop": "c"}, Head: "develop"},
			want: []ChannelInfo{
				{Name: "develop", Kind: "pointer", Latest: "develop", IsDefaultBranchFallback: true},
			},
		},
		{
			name: "no tags and no head",
			snap: domain.RefSnapshot{Branches: map[string]string{"develop": "c"}},
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ChannelsOf(tc.snap))
		})
	}
}

func TestChannelsOf_DeterministicAcrossCalls(t *testing.T) {
	snap := domain.RefSnapshot{Tags: map[string]string{"v1.0": "a", "1.0": "b", "v1.0.0": "c"}}
	first := ChannelsOf(snap)
	for i := 0; i < 20; i++ {
		require.Equal(t, first, ChannelsOf(snap))
	}
}

func TestListChannels_SnapshotError_Propagates(t *testing.T) {
	refsErr := errors.New("dial tcp: connection refused")
	crs := &stubConstraintResolver{refsErr: refsErr}
	m := snapshotManifold(crs, &fakeClock{now: time.Now()})

	_, err := m.ListChannels(context.Background(), domain.Namespace("github.com/u/r"))
	assert.ErrorIs(t, err, refsErr)
}
