package snapshot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type stubRefs struct {
	refs     *domain.RefSnapshot
	refsErr  error
	refsCall int
}

func (s *stubRefs) Refs(_ context.Context, _ domain.Namespace) (domain.RefSnapshot, error) {
	s.refsCall++
	if s.refsErr != nil {
		return domain.RefSnapshot{}, s.refsErr
	}
	return *s.refs, nil
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func newSnapshots(
	refs *stubRefs,
	clock *fakeClock,
) Snapshots {
	return New(refs, clock.Now, time.Hour)
}

func sharedSnapshot() domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags: map[string]string{
			"v1.2.0":         "c-v1.2.0",
			"v1.3.0":         "c-v1.3.0",
			"beta-26.5-4":    "c-beta-4",
			"beta-26.5-1":    "c-beta-1",
			"stable-26.5.1":  "c-stable-26.5.1",
			"nightly":        "c-nightly",
			"nightly-latest": "c-nightly-latest",
			"v1.0-latest":    "c-v1.0-latest",
		},
		Branches: map[string]string{"develop": "c-develop", "deadbeef": "c-deadbeef"},
		Head:     "develop",
	}
}

func TestSnapshot_SecondCallServedFromCache(t *testing.T) {
	snap := sharedSnapshot()
	crs := &stubRefs{refs: &snap}
	m := newSnapshots(crs, &fakeClock{now: time.Now()})
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
	crs := &stubRefs{refs: &before}
	now := time.Now()
	clock := &fakeClock{now: now}
	m := newSnapshots(crs, clock)
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
	crs := &stubRefs{refsErr: refsErr}
	m := newSnapshots(crs, &fakeClock{now: time.Now()})
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

// An update re-resolves right before it starts and again before it commits;
// a snapshot cached for the version-check TTL would hide a tag that moved in
// between, so FreshSnapshot always reads the remote and refreshes the cache.
func TestFreshSnapshot_BypassesAndRefreshesTheCache(t *testing.T) {
	before := domain.RefSnapshot{Tags: map[string]string{"nightly": "old"}}
	crs := &stubRefs{refs: &before}
	m := newSnapshots(crs, &fakeClock{now: time.Now()})
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
	crs := &stubRefs{refs: &snap}
	m := newSnapshots(crs, &fakeClock{now: time.Now()})
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
