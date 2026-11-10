package collection

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/bind"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func hostedAsynx(t *testing.T, dir string) bind.Bound[domain.Collection] {
	t.Helper()
	es, err := sqlite.NewEventStore(filepath.Join(dir, "collection.db"))
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(filepath.Join(dir, "collection_snapshots.db"))
	require.NoError(t, err)
	ax, err := bind.Build(asynx.New[domain.Collection]().
		WithEventStore(es).WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 2, QueueDepth: 100}), es)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ax.Shutdown(context.Background())
		_ = es.Close()
		_ = ss.Close()
	})
	return ax
}

func TestHostedCollection_FollowIsReadableTheMomentSendReturns(t *testing.T) {
	dir := t.TempDir()
	ax := hostedAsynx(t, dir)

	repo, err := NewFromDBPath(ax, filepath.Join(dir, "collections.db"), &mocks.Vault{}, &mocks.Manifold{})
	require.NoError(t, err)
	ax.Ready()

	announced := make(chan domain.Collection, 1)
	require.NoError(t, repo.OnCollectionFollowed(func(_ context.Context, c domain.Collection) { announced <- c }))

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, repo.Follow(context.Background(), ns, makeTestManifest("q"), nil))

	listed, err := repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, listed, 1, "the sync projection committed the row with the event")
	assert.Equal(t, ns, listed[0].Namespace)
	assert.Equal(t, ns, (<-announced).Namespace, "the durable subscriber announces it afterwards")
}

func TestHostedCollection_ImportsTheLegacyReadModel(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "collections.db")
	legacy, err := store.New(legacyPath)
	require.NoError(t, err)
	ns := domain.Namespace("github.com/org/old")
	require.NoError(t, legacy.Save(context.Background(), domain.Collection{Namespace: ns}))
	require.NoError(t, legacy.Close())

	ax := hostedAsynx(t, dir)
	repo, err := NewFromDBPath(ax, legacyPath, &mocks.Vault{}, &mocks.Manifold{})
	require.NoError(t, err)
	ax.Ready()

	listed, err := repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, ns, listed[0].Namespace)
	_, statErr := os.Stat(legacyPath + ".migrated")
	assert.NoError(t, statErr)
}

func TestNewFromDBPath_HostedStoreFailure_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	ax := hostedAsynx(t, dir)
	garbage := filepath.Join(dir, "collections.db")
	require.NoError(t, os.WriteFile(garbage, []byte("this is not a sqlite database at all, just text"), 0o600))

	_, err := NewFromDBPath(ax, garbage, &mocks.Vault{}, &mocks.Manifold{})
	require.Error(t, err)
}
