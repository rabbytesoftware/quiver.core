package device_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/device"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func newTestAsynx(t *testing.T) asynx.Asynx[auth.Device] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[auth.Device]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 2, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func newTestDB(t *testing.T) *gormdb.DB {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapterSQLite.CloseDB(db) })
	return db
}

func newTestRepo(t *testing.T) device.Device {
	t.Helper()
	repo, err := device.New(newTestDB(t), newTestAsynx(t))
	require.NoError(t, err)
	return repo
}

func TestNew_NilAsynx_ReturnsError(t *testing.T) {
	_, err := device.New(newTestDB(t), nil)
	assert.Error(t, err)
}

func TestNew_StoreConstructionFails_ReturnsError(t *testing.T) {
	_, err := device.New(nil, newTestAsynx(t))
	assert.Error(t, err)
}

func TestPair_NewDevice_BecomesActiveAndReadable(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-1"))

	got, err := repo.Get(ctx, "dev-1")
	require.NoError(t, err)
	assert.Equal(t, auth.DeviceStateActive, got.State)
	assert.Equal(t, "laptop", got.Label)
}

func TestPair_ExistingDevice_RotatesToken(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-1"))
	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-2"))

	_, err := repo.Authenticate(ctx, "hash-1")
	assert.ErrorIs(t, err, apperrors.ErrUnauthorized)

	got, err := repo.Authenticate(ctx, "hash-2")
	require.NoError(t, err)
	assert.Equal(t, "dev-1", got.ID)
}

func TestRevoke_ActiveDevice_DeactivatesCredential(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-1"))

	require.NoError(t, repo.Revoke(ctx, "dev-1"))

	_, err := repo.Authenticate(ctx, "hash-1")
	assert.ErrorIs(t, err, apperrors.ErrUnauthorized)

	got, err := repo.Get(ctx, "dev-1")
	require.NoError(t, err)
	assert.Equal(t, auth.DeviceStateRevoked, got.State)
}

func TestRevoke_UnknownDevice_ReturnsNotFound(t *testing.T) {
	repo := newTestRepo(t)
	err := repo.Revoke(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRevoke_AlreadyRevoked_ReturnsNotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-1"))
	require.NoError(t, repo.Revoke(ctx, "dev-1"))

	err := repo.Revoke(ctx, "dev-1")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestTouch_ActiveDevice_UpdatesLastSeenAt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-1"))

	before, err := repo.Get(ctx, "dev-1")
	require.NoError(t, err)

	require.NoError(t, repo.Touch(ctx, "dev-1"))

	after, err := repo.Get(ctx, "dev-1")
	require.NoError(t, err)
	assert.True(t, after.LastSeenAt.After(before.LastSeenAt) || after.LastSeenAt.Equal(before.LastSeenAt))
}

func TestTouch_UnknownDevice_ReturnsNotFound(t *testing.T) {
	repo := newTestRepo(t)
	err := repo.Touch(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAuthenticate_UnknownHash_ReturnsUnauthorized(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.Authenticate(context.Background(), "missing-hash")
	assert.ErrorIs(t, err, apperrors.ErrUnauthorized)
}

func TestGet_UnknownDevice_ReturnsNotFound(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestList_ReturnsPairedDevices(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Pair(ctx, "dev-1", "laptop", "hash-1"))
	require.NoError(t, repo.Pair(ctx, "dev-2", "phone", "hash-2"))

	devices, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, devices, 2)
}

func TestShutdown_DrainsAsynx(t *testing.T) {
	repo := newTestRepo(t)
	assert.NoError(t, repo.Shutdown(context.Background()))
}
