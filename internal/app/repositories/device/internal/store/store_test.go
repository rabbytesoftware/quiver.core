package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	devicestore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/device/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func newTestDB(t *testing.T) *gormdb.DB {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapterSQLite.CloseDB(db) })
	return db
}

func TestNew_NilDB_ReturnsError(t *testing.T) {
	_, err := devicestore.New(nil)
	assert.Error(t, err)
}

func TestUpsertAndGet_RoundTrips(t *testing.T) {
	st, err := devicestore.New(newTestDB(t))
	require.NoError(t, err)

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	d := auth.Device{
		ID: "dev-1", Label: "laptop", TokenHash: "hash-1",
		State: auth.DeviceStateActive, PairedAt: now, LastSeenAt: now,
	}

	ctx := context.Background()
	require.NoError(t, st.Upsert(ctx, d))

	got, err := st.Get(ctx, "dev-1")
	require.NoError(t, err)
	assert.Equal(t, d.ID, got.ID)
	assert.Equal(t, d.Label, got.Label)
	assert.Equal(t, d.TokenHash, got.TokenHash)
	assert.Equal(t, d.State, got.State)
	assert.True(t, d.PairedAt.Equal(got.PairedAt))
}

func TestUpsert_OverwritesExistingRow(t *testing.T) {
	st, err := devicestore.New(newTestDB(t))
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now()
	require.NoError(t, st.Upsert(ctx, auth.Device{ID: "dev-1", TokenHash: "hash-1", State: auth.DeviceStateActive, PairedAt: now, LastSeenAt: now}))
	require.NoError(t, st.Upsert(ctx, auth.Device{ID: "dev-1", TokenHash: "hash-2", State: auth.DeviceStateActive, PairedAt: now, LastSeenAt: now}))

	got, err := st.Get(ctx, "dev-1")
	require.NoError(t, err)
	assert.Equal(t, "hash-2", got.TokenHash)

	devices, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
}

func TestGet_Unknown_ReturnsNotFound(t *testing.T) {
	st, err := devicestore.New(newTestDB(t))
	require.NoError(t, err)

	_, err = st.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetByTokenHash_Found(t *testing.T) {
	st, err := devicestore.New(newTestDB(t))
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now()
	require.NoError(t, st.Upsert(ctx, auth.Device{ID: "dev-1", TokenHash: "hash-1", State: auth.DeviceStateActive, PairedAt: now, LastSeenAt: now}))

	got, err := st.GetByTokenHash(ctx, "hash-1")
	require.NoError(t, err)
	assert.Equal(t, "dev-1", got.ID)
}

func TestGetByTokenHash_Unknown_ReturnsNotFound(t *testing.T) {
	st, err := devicestore.New(newTestDB(t))
	require.NoError(t, err)

	_, err = st.GetByTokenHash(context.Background(), "missing-hash")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestList_ReturnsAllDevicesNewestFirst(t *testing.T) {
	st, err := devicestore.New(newTestDB(t))
	require.NoError(t, err)

	ctx := context.Background()
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	require.NoError(t, st.Upsert(ctx, auth.Device{ID: "dev-1", TokenHash: "h1", PairedAt: older, LastSeenAt: older}))
	require.NoError(t, st.Upsert(ctx, auth.Device{ID: "dev-2", TokenHash: "h2", PairedAt: newer, LastSeenAt: newer}))

	devices, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, "dev-2", devices[0].ID)
}
