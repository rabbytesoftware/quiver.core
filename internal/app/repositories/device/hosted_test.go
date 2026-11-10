package device_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/bind"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/device"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func hostedDeviceAsynx(t *testing.T) bind.Bound[auth.Device] {
	t.Helper()
	dir := t.TempDir()
	es, err := sqlite.NewEventStore(filepath.Join(dir, "device.db"))
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(filepath.Join(dir, "device_snapshots.db"))
	require.NoError(t, err)
	ax, err := bind.Build(asynx.New[auth.Device]().
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

func TestHostedDevice_ReadModelCommitsWithTheEvent(t *testing.T) {
	ax := hostedDeviceAsynx(t)
	repo, err := device.New(nil, ax)
	require.NoError(t, err)
	ax.Ready()
	ctx := context.Background()

	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-1"))
	got, err := repo.Authenticate(ctx, "hash-1")
	require.NoError(t, err)
	assert.Equal(t, "dev-1", got.ID)

	require.NoError(t, repo.Revoke(ctx, "dev-1"))
	_, err = repo.Authenticate(ctx, "hash-1")
	require.Error(t, err, "a revoked device authenticates nothing the moment Revoke returns")
}

func TestHostedDevice_ImportsTheLegacyDatabase(t *testing.T) {
	legacy, err := adapterSQLite.OpenDB(filepath.Join(t.TempDir(), "device_store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapterSQLite.CloseDB(legacy) })
	old, err := device.New(legacy, newTestAsynx(t))
	require.NoError(t, err)
	require.NoError(t, old.Pair(context.Background(), "dev-old", "phone", "hash-old"))

	ax := hostedDeviceAsynx(t)
	repo, err := device.New(legacy, ax)
	require.NoError(t, err)
	ax.Ready()

	got, err := repo.Authenticate(context.Background(), "hash-old")
	require.NoError(t, err)
	assert.Equal(t, "dev-old", got.ID, "a device paired before the upgrade still authenticates")
}
