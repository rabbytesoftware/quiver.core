package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

type steppedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *steppedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *steppedClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type refsFixture struct {
	dir   string
	clock *steppedClock
	start time.Time
}

func newRefsFixture(t *testing.T) *refsFixture {
	t.Helper()
	now := time.Now()
	return &refsFixture{dir: t.TempDir(), clock: &steppedClock{now: now}, start: now}
}

// open builds a store over the fixture's vault directory, as a daemon start
// does: nothing in memory, whatever an earlier one saved on disk.
func (f *refsFixture) open(
	t *testing.T,
	m *mocks.Manifold,
	opts ...store.Option,
) store.Store {
	t.Helper()
	v, err := vault.NewWithClock(
		filepath.Join(f.dir, "vault"),
		filepath.Join(f.dir, "ns"),
		time.Hour,
		f.clock.Now,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.NewWithClock(db, v, m, f.clock.Now, opts...)
	require.NoError(t, err)
	return r
}

func TestRefs_NeverSaved_WaitsOnTheHostOnceAndSavesTheAnswer(t *testing.T) {
	f := newRefsFixture(t)
	m := &mocks.Manifold{SnapshotResult: selectorSnapshot()}
	r := f.open(t, m)

	first, err := r.Refs(context.Background(), selectorBare.WithRef("stable"))
	require.NoError(t, err)
	second, err := r.Refs(context.Background(), selectorBare)
	require.NoError(t, err)

	assert.Equal(t, selectorSnapshot(), first)
	assert.Equal(t, first, second)
	assert.Equal(t, 1, m.SnapshotCalls, "the second view is answered from what the first saved")
}

func TestRefs_AfterARestart_AnswersFromDiskWithoutTheHost(t *testing.T) {
	f := newRefsFixture(t)
	_, err := f.open(t, &mocks.Manifold{SnapshotResult: selectorSnapshot()}).Refs(context.Background(), selectorBare)
	require.NoError(t, err)

	hostDown := &mocks.Manifold{SnapshotErr: errors.New("host unreachable")}
	f.clock.Advance(10 * time.Minute)
	got, err := f.open(t, hostDown).Refs(context.Background(), selectorBare)

	require.NoError(t, err)
	assert.Equal(t, selectorSnapshot(), got)
	assert.Zero(t, hostDown.SnapshotCalls+hostDown.FreshSnapshotCalls)
}

func TestRefs_Expired_IsServedThenReReadAndReported(t *testing.T) {
	f := newRefsFixture(t)
	old := selectorSnapshot()
	moved := selectorSnapshot()
	moved.Tags["v3.0.0"] = "c300"
	_, err := f.open(t, &mocks.Manifold{SnapshotResult: old}).Refs(context.Background(), selectorBare)
	require.NoError(t, err)

	reported := make(chan domain.Arrow, 1)
	m := &mocks.Manifold{SnapshotResult: moved}
	r := f.open(t, m, store.WithRefreshed(func(a domain.Arrow) { reported <- a }))
	f.clock.Advance(2 * time.Hour)

	got, err := r.Refs(context.Background(), selectorBare)
	require.NoError(t, err)
	assert.Equal(t, old, got, "the expired copy answers before the host does")

	store.WaitRefs(r)
	arrow := <-reported
	assert.Equal(t, selectorBare, arrow.Namespace)
	assert.False(t, arrow.UserInstalled)
	assert.Equal(t, 1, m.FreshSnapshotCalls)
	again, err := r.Refs(context.Background(), selectorBare)
	require.NoError(t, err)
	assert.Equal(t, moved, again)
}

func TestRefs_ReReadFindingNothingNew_ReportsNothing(t *testing.T) {
	f := newRefsFixture(t)
	_, err := f.open(t, &mocks.Manifold{SnapshotResult: selectorSnapshot()}).Refs(context.Background(), selectorBare)
	require.NoError(t, err)

	reported := make(chan domain.Arrow, 1)
	m := &mocks.Manifold{SnapshotResult: selectorSnapshot()}
	r := f.open(t, m, store.WithRefreshed(func(a domain.Arrow) { reported <- a }))
	f.clock.Advance(2 * time.Hour)

	_, err = r.Refs(context.Background(), selectorBare)
	require.NoError(t, err)
	store.WaitRefs(r)

	assert.Equal(t, 1, m.FreshSnapshotCalls)
	assert.Empty(t, reported)
}

func TestRefs_FailedReRead_KeepsServingTheSavedCopy(t *testing.T) {
	f := newRefsFixture(t)
	_, err := f.open(t, &mocks.Manifold{SnapshotResult: selectorSnapshot()}).Refs(context.Background(), selectorBare)
	require.NoError(t, err)

	m := &mocks.Manifold{SnapshotErr: errors.New("host unreachable")}
	r := f.open(t, m)
	f.clock.Advance(2 * time.Hour)

	for range 2 {
		got, refsErr := r.Refs(context.Background(), selectorBare)
		require.NoError(t, refsErr)
		assert.Equal(t, selectorSnapshot(), got)
		store.WaitRefs(r)
	}
}

func TestRefs_NeverSavedAndHostDown_ReportsTheError(t *testing.T) {
	f := newRefsFixture(t)
	hostDown := errors.New("host unreachable")
	r := f.open(t, &mocks.Manifold{SnapshotErr: hostDown})

	_, err := r.Refs(context.Background(), selectorBare)

	require.ErrorIs(t, err, hostDown)
}

func TestRefs_SaveFailing_StillAnswers(t *testing.T) {
	v := &mocks.Vault{PutRefsErr: errors.New("disk full")}
	r := newTestReaderWithVaultManifold(t, v, &mocks.Manifold{SnapshotResult: selectorSnapshot()})

	got, err := r.Refs(context.Background(), selectorBare)

	require.NoError(t, err)
	assert.Equal(t, selectorSnapshot(), got)
	assert.Equal(t, 1, v.PutRefsCalls)
}

func TestRefs_NoVault_ReadsTheHost(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	m := &mocks.Manifold{SnapshotResult: selectorSnapshot()}
	r, err := store.New(db, nil, m)
	require.NoError(t, err)

	got, err := r.Refs(context.Background(), selectorBare)

	require.NoError(t, err)
	assert.Equal(t, selectorSnapshot(), got)
	assert.Equal(t, 1, m.SnapshotCalls)
}
