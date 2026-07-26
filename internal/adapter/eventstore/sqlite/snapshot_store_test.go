package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestSnapshotStore(t *testing.T) SnapshotStore {
	t.Helper()
	s, err := NewSnapshotStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSnapshotStore_Get_NotFoundReturnsFoundFalse(t *testing.T) {
	s := newTestSnapshotStore(t)

	data, found, err := s.Get(context.Background(), "missing")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, data)
}

func TestSnapshotStore_PutThenGet_RoundTrips(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "agg-1", 1, []byte(`{"v":1}`)))

	data, found, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, `{"v":1}`, string(data))
}

func TestSnapshotStore_Put_UpsertReplacesRatherThanDuplicates(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "agg-1", 1, []byte(`{"v":1}`)))
	require.NoError(t, s.Put(ctx, "agg-1", 2, []byte(`{"v":2}`)))

	data, found, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, `{"v":2}`, string(data))
}

func TestSnapshotStore_Put_StaleVersionIsNoOp(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "agg-1", 5, []byte(`{"v":5}`)))
	require.NoError(t, s.Put(ctx, "agg-1", 3, []byte(`{"v":3}`)))

	data, _, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v":5}`, string(data), "the monotonicity guard must not let an older snapshot overwrite a newer one")
}

func TestSnapshotStore_Put_IsolatesAggregates(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "agg-1", 1, []byte(`{"id":"a"}`)))
	require.NoError(t, s.Put(ctx, "agg-2", 1, []byte(`{"id":"b"}`)))

	a, _, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	b, _, err := s.Get(ctx, "agg-2")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"a"}`, string(a))
	assert.JSONEq(t, `{"id":"b"}`, string(b))
}

func TestSnapshotStore_Delete_RemovesSnapshot(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "agg-1", 1, []byte(`{"v":1}`)))
	require.NoError(t, s.Delete(ctx, "agg-1"))

	_, found, err := s.Get(ctx, "agg-1")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSnapshotStore_Delete_MissingAggregateIsNotAnError(t *testing.T) {
	s := newTestSnapshotStore(t)

	assert.NoError(t, s.Delete(context.Background(), "never-existed"))
}

func TestSnapshotStore_CancelledContext(t *testing.T) {
	s := newTestSnapshotStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	testCases := []struct {
		name string
		call func() error
	}{
		{"put", func() error { return s.Put(ctx, "agg-1", 1, []byte(`{}`)) }},
		{"get", func() error { _, _, err := s.Get(ctx, "agg-1"); return err }},
		{"delete", func() error { return s.Delete(ctx, "agg-1") }},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, tc.call())
		})
	}
}

func TestNewSnapshotStore_InvalidPathReturnsError(t *testing.T) {
	_, err := NewSnapshotStore(filepath.Join(t.TempDir(), "no-such-dir", "s.db"))
	assert.Error(t, err)
}

func TestNewSnapshotStore_ReadOnlyFileReturnsError(
	t *testing.T,
) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}

	path := filepath.Join(t.TempDir(), "readonly.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	require.NoError(t, os.Chmod(path, 0o444))

	_, err = NewSnapshotStore(path)
	assert.Error(t, err)
}

func TestNewSnapshotStore_ConflictingSchemaReturnsError(
	t *testing.T,
) {
	path := filepath.Join(t.TempDir(), "conflict.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE VIEW snapshots AS SELECT 1 AS aggregate_id").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = NewSnapshotStore(path)
	assert.Error(t, err)
}
