package store_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

func openHeld(
	t *testing.T,
	v vault.Vault,
	m *mocks.Manifold,
	opts ...store.Option,
) store.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.New(db, v, m, opts...)
	require.NoError(t, err)
	return r
}

// filedByAPreview is a vault holding what a first, live view of selectorBare
// filed: the manifest under its channel identity.
// parsing makes m parse any manifest as the arrow it was filed for.
func parsing(m *mocks.Manifold) *mocks.Manifold {
	m.ParseArrowFn = func([]byte) (*domain.Arrow, error) {
		return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "crowbar"}}, nil
	}
	return m
}

func filedByAPreview(t *testing.T) vault.Vault {
	t.Helper()
	var fetched []commitFetch
	v := realVault(t)
	_, err := openHeld(t, v, selectorManifold(selectorSnapshot(), &fetched)).ResolveManifest(context.Background(), selectorBare)
	require.NoError(t, err)
	return v
}

func TestResolveManifest_Refless_AfterARestartIsServedFromTheVaultWhileTheHostIsDown(t *testing.T) {
	v := filedByAPreview(t)
	hostDown := &mocks.Manifold{
		SnapshotErr: errors.New("host unreachable"),
	}
	r := openHeld(t, v, parsing(hostDown))

	got, err := r.ResolveManifest(context.Background(), selectorBare)
	store.WaitRechecks(r)

	require.NoError(t, err)
	assert.Equal(t, "crowbar", got.Name)
	assert.Equal(t, selectorBare.WithRef("stable"), got.Namespace, "the channel the namespace follows, not the tag it stands at")
	assert.Equal(t, "v2.0.0", got.Resolved.Ref)
	assert.Equal(t, "c200", got.Resolved.Commit)
	assert.Equal(t, 1, hostDown.SnapshotCalls, "the host is asked behind the answer")
}

func TestResolveManifest_Refless_RecheckFindingANewerCommit_IsReported(t *testing.T) {
	v := filedByAPreview(t)
	moved := selectorSnapshot()
	moved.Tags["v3.0.0"] = "c300"
	var fetched []commitFetch
	reported := make(chan domain.Arrow, 1)
	r := openHeld(t, v, parsing(selectorManifold(moved, &fetched)), store.WithRefreshed(func(a domain.Arrow) { reported <- a }))

	got, err := r.ResolveManifest(context.Background(), selectorBare)
	store.WaitRechecks(r)

	require.NoError(t, err)
	assert.Equal(t, "c200", got.Resolved.Commit, "the held copy answers first")
	select {
	case arrow := <-reported:
		assert.Equal(t, selectorBare.WithRef("stable"), arrow.Namespace)
		assert.Equal(t, "c300", arrow.Resolved.Commit)
		assert.False(t, arrow.UserInstalled)
	case <-time.After(5 * time.Second):
		t.Fatal("the newer commit was never reported")
	}
}

func TestResolveManifest_Refless_RecheckFindingNothingNew_ReportsNothing(t *testing.T) {
	v := filedByAPreview(t)
	var fetched []commitFetch
	reported := make(chan domain.Arrow, 1)
	r := openHeld(t, v, parsing(selectorManifold(selectorSnapshot(), &fetched)), store.WithRefreshed(func(a domain.Arrow) { reported <- a }))

	_, err := r.ResolveManifest(context.Background(), selectorBare)
	store.WaitRechecks(r)

	require.NoError(t, err)
	assert.Empty(t, reported)
}

func TestResolveManifest_Refless_UnparsableHeldCopy_TakesTheLivePath(t *testing.T) {
	v := filedByAPreview(t)
	var fetched []commitFetch
	m := selectorManifold(selectorSnapshot(), &fetched)
	m.ParseArrowFn = func([]byte) (*domain.Arrow, error) { return nil, errors.New("bad manifest") }

	got, err := openHeld(t, v, m).ResolveManifest(context.Background(), selectorBare)

	require.NoError(t, err)
	assert.Equal(t, "crowbar", got.Name)
	assert.Equal(t, 1, m.SnapshotCalls, "nothing usable was held, so the host is asked before answering")
	assert.Len(t, fetched, 1)
}

// A page opens its detail, manifest, readme and dependencies at once; for a
// repository nothing holds yet they share one resolution instead of each
// repeating the same fetches.
func TestResolveManifest_ConcurrentColdViews_ShareOneResolution(t *testing.T) {
	var fetches atomic.Int32
	release := make(chan struct{})
	m := parsing(&mocks.Manifold{
		SnapshotResult: selectorSnapshot(),
		ResolveArrowAtCommitFn: func(_ context.Context, ns domain.Namespace, _, _ string) (*domain.Arrow, []byte, string, error) {
			fetches.Add(1)
			<-release
			return &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "crowbar"}}, []byte("raw"), "ARROW.md", nil
		},
	})
	r := openHeld(t, realVault(t), m)

	const views = 6
	results := make([]*domain.Arrow, views)
	var wg sync.WaitGroup
	for i := range views {
		wg.Add(1)
		go func() {
			defer wg.Done()
			arrow, err := r.ResolveManifest(context.Background(), selectorBare)
			assert.NoError(t, err)
			results[i] = arrow
		}()
	}
	require.Eventually(t, func() bool { return fetches.Load() == 1 }, 5*time.Second, time.Millisecond)
	close(release)
	wg.Wait()
	store.WaitRechecks(r)

	assert.Equal(t, int32(1), fetches.Load())
	for _, arrow := range results {
		require.NotNil(t, arrow)
		assert.Equal(t, "crowbar", arrow.Name)
	}
	assert.NotSame(t, results[0], results[1], "each view gets its own copy")
}

func TestResolveManifest_ColdViewFailing_ReportsTheErrorToEveryWaiter(t *testing.T) {
	hostDown := errors.New("host unreachable")
	r := openHeld(t, realVault(t), &mocks.Manifold{SnapshotErr: hostDown})

	_, err := r.ResolveManifest(context.Background(), selectorBare)

	require.ErrorIs(t, err, hostDown)
}
