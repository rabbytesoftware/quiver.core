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
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
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
	r.Wait()

	require.NoError(t, err)
	assert.Equal(t, "crowbar", got.Name)
	assert.Equal(t, selectorBare.WithRef("stable"), got.Namespace, "the channel the namespace follows, not the tag it stands at")
	assert.Equal(t, "v2.0.0", got.Resolved.Ref)
	assert.Equal(t, "c200", got.Resolved.Commit)
	assert.Equal(t, 1, hostDown.FreshSnapshotCalls, "the host is asked live behind the answer")
}

func TestResolveManifest_Refless_RecheckFindingANewerCommit_IsReported(t *testing.T) {
	v := filedByAPreview(t)
	moved := selectorSnapshot()
	moved.Tags["v3.0.0"] = "c300"
	var fetched []commitFetch
	reported := make(chan domain.Arrow, 1)
	r := openHeld(t, v, parsing(selectorManifold(moved, &fetched)), store.WithRefreshed(func(a domain.Arrow) { reported <- a }))

	got, err := r.ResolveManifest(context.Background(), selectorBare)
	r.Wait()

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
	r.Wait()

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
	r.Wait()

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

func TestStore_Stop_CancelsARecheckStuckOnTheHost(t *testing.T) {
	v := filedByAPreview(t)
	asked := make(chan struct{})
	stuck := parsing(&mocks.Manifold{
		SnapshotFn: func(ctx context.Context, _ domain.Namespace) (domain.RefSnapshot, error) {
			close(asked)
			<-ctx.Done()
			return domain.RefSnapshot{}, ctx.Err()
		},
	})
	r := openHeld(t, v, stuck)
	_, err := r.ResolveManifest(context.Background(), selectorBare)
	require.NoError(t, err)
	<-asked

	stopped := make(chan struct{})
	go func() {
		r.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop waited out the recheck instead of cancelling it")
	}
}

func filedWithChannels(t *testing.T) vault.Vault {
	t.Helper()
	v := realVault(t)
	var fetched []commitFetch
	m := selectorManifold(selectorSnapshot(), &fetched)
	m.ListChannelsResult = []manifold.ChannelInfo{{Name: "stable", Kind: "ordered", Latest: "v2.0.0", Count: 2}}
	_, err := openHeld(t, v, m).ResolveManifest(context.Background(), selectorBare)
	require.NoError(t, err)
	return v
}

func TestStore_HeldChannels_AfterARestartAreServedFromTheVaultWhileTheHostIsDown(t *testing.T) {
	v := filedWithChannels(t)
	hostDown := parsing(&mocks.Manifold{SnapshotErr: errors.New("host unreachable")})
	r := openHeld(t, v, hostDown)

	got, ok := r.HeldChannels(context.Background(), selectorBare)
	r.Wait()

	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "stable", got[0].Name)
	assert.Equal(t, "v2.0.0", got[0].Latest)
	assert.Equal(t, 1, hostDown.FreshSnapshotCalls, "the host is asked live behind the answer")
}

func TestStore_HeldChannels_NothingFiledWithThem_AnswersNothing(t *testing.T) {
	v := filedByAPreview(t)
	r := openHeld(t, v, parsing(&mocks.Manifold{}))

	_, ok := r.HeldChannels(context.Background(), selectorBare)

	assert.False(t, ok)
}

func TestStore_HeldChannels_ASecondDefaultMark_TheNewestEntryWins(t *testing.T) {
	v := realVault(t)
	older := selectorBare.WithRef("stable")
	newer := selectorBare.WithRef("nightly")
	clock := time.Now()
	for i, tc := range []struct {
		ns   domain.Namespace
		name string
	}{{older, "stable"}, {newer, "nightly"}} {
		raw := []byte(`[{"Name":"` + tc.name + `"}]`)
		require.NoError(t, v.PutArrow(context.Background(), tc.ns, vault.ManifestFile{
			Content: []byte("raw"), Filename: "ARROW.md", Ref: "v1.0." + string(rune('0'+i)), Commit: "c" + tc.name, Default: true, Channels: raw,
		}))
		clock = clock.Add(time.Second)
		time.Sleep(10 * time.Millisecond)
	}
	r := openHeld(t, v, parsing(&mocks.Manifold{}))

	got, ok := r.HeldChannels(context.Background(), selectorBare)
	r.Wait()

	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "nightly", got[0].Name)
}

func TestStore_HeldChannels_AnOlderDefaultRewrittenByARefresh_DoesNotOutrankTheCurrentOne(t *testing.T) {
	v := realVault(t)
	ctx := context.Background()
	put := func(ns domain.Namespace, name string, isDefault bool) {
		require.NoError(t, v.PutArrow(ctx, ns, vault.ManifestFile{
			Content: []byte("raw"), Filename: "ARROW.md", Ref: "v1.0.0", Commit: "c" + name, Default: isDefault,
			Channels: []byte(`[{"Name":"` + name + `"}]`),
		}))
		time.Sleep(10 * time.Millisecond)
	}
	older := selectorBare.WithRef("stable")
	current := selectorBare.WithRef("nightly")
	put(older, "stable", true)
	put(current, "nightly", true)
	put(older, "stable", false)
	r := openHeld(t, v, parsing(&mocks.Manifold{}))

	got, ok := r.HeldChannels(ctx, selectorBare)
	r.Wait()

	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "nightly", got[0].Name)
}
