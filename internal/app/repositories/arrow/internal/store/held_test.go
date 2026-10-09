package store_test

import (
	"context"
	"errors"
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
	m.ParseArrowResult = &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "crowbar"}}
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
		SnapshotErr:      errors.New("host unreachable"),
		ParseArrowResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "crowbar"}},
	}
	r := openHeld(t, v, hostDown)

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
	m.ParseArrowResult, m.ParseArrowErr = nil, errors.New("bad manifest")

	got, err := openHeld(t, v, m).ResolveManifest(context.Background(), selectorBare)

	require.NoError(t, err)
	assert.Equal(t, "crowbar", got.Name)
	assert.Equal(t, 1, m.SnapshotCalls, "nothing usable was held, so the host is asked before answering")
	assert.Len(t, fetched, 1)
}
