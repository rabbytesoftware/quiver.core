package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNewEventStore_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewEventStore("/nonexistent-dir-quiver-test/db.sqlite")
	assert.Error(t, err)
}

func TestNewEventStore_ReadOnlyFileReturnsError(
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

	_, err = NewEventStore(path)
	assert.Error(t, err)
}

func TestNewEventStore_ConflictingSchemaReturnsError(
	t *testing.T,
) {
	path := filepath.Join(t.TempDir(), "conflict.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE VIEW events AS SELECT 1 AS aggregate_id").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = NewEventStore(path)
	assert.Error(t, err)
}

func TestDelete_ContextCancelled_ReturnsError(t *testing.T) {
	es := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, es.Append(ctx, "agg-cancel", 0, []byte("data")))

	err := es.Delete(cancelledCtx(), "agg-cancel")
	assert.Error(t, err)
}

func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func newTestEventStore(t *testing.T) Store {
	t.Helper()
	s, err := NewEventStore(":memory:")
	require.NoError(t, err)
	return s
}

func TestEventStore_Append_Success(
	t *testing.T,
) {
	s := newTestEventStore(t)
	ctx := context.Background()

	err := s.Append(ctx, "agg-1", 1, []byte("event1"))
	require.NoError(t, err)

	blobs, err := s.ReadFrom(ctx, "agg-1", 1)
	require.NoError(t, err)
	require.Len(t, blobs, 1)
	assert.Equal(t, []byte("event1"), blobs[0])
}

func TestEventStore_Append_VersionConflict(
	t *testing.T,
) {
	s := newTestEventStore(t)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("first")))

	err := s.Append(ctx, "agg-1", 1, []byte("duplicate"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrPipelineFailed))
}

func TestEventStore_Append_ContextCancelled(
	t *testing.T,
) {
	s := newTestEventStore(t)
	err := s.Append(cancelledCtx(), "agg-1", 1, []byte("data"))
	assert.Error(t, err)
}

func TestEventStore_ReadFrom_Success(
	t *testing.T,
) {
	s := newTestEventStore(t)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	require.NoError(t, s.Append(ctx, "agg-1", 3, []byte("e3")))

	blobs, err := s.ReadFrom(ctx, "agg-1", 2)
	require.NoError(t, err)
	require.Len(t, blobs, 2)
	assert.Equal(t, []byte("e2"), blobs[0])
	assert.Equal(t, []byte("e3"), blobs[1])
}

func TestEventStore_ReadFrom_EmptyStream(
	t *testing.T,
) {
	s := newTestEventStore(t)
	blobs, err := s.ReadFrom(context.Background(), "nonexistent", 1)
	require.NoError(t, err)
	assert.Empty(t, blobs)
}

func TestEventStore_ReadFrom_ContextCancelled(
	t *testing.T,
) {
	s := newTestEventStore(t)
	_, err := s.ReadFrom(cancelledCtx(), "agg-1", 1)
	assert.Error(t, err)
}

func TestEventStore_ReadRange_Success(
	t *testing.T,
) {
	s := newTestEventStore(t)
	ctx := context.Background()

	for i := int64(1); i <= 5; i++ {
		require.NoError(t, s.Append(ctx, "agg-1", i, []byte("data")))
	}

	blobs, err := s.ReadRange(ctx, "agg-1", 1, 3)
	require.NoError(t, err)
	assert.Len(t, blobs, 3)
}

func TestEventStore_ReadRange_TruncatesCount(
	t *testing.T,
) {
	s := newTestEventStore(t)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))

	blobs, err := s.ReadRange(ctx, "agg-1", 1, 100)
	require.NoError(t, err)
	assert.Len(t, blobs, 2)
}

func TestEventStore_ReadRange_ContextCancelled(
	t *testing.T,
) {
	s := newTestEventStore(t)
	_, err := s.ReadRange(cancelledCtx(), "agg-1", 1, 10)
	assert.Error(t, err)
}

func TestDelete_RemovesAllEntriesForAggregate(t *testing.T) {
	es := newTestEventStore(t)

	ctx := context.Background()
	require.NoError(t, es.Append(ctx, "agg-1", 0, []byte("e0")))
	require.NoError(t, es.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, es.Append(ctx, "agg-2", 0, []byte("e0")))

	require.NoError(t, es.Delete(ctx, "agg-1"))

	entries, err := es.ReadFrom(ctx, "agg-1", 0)
	require.NoError(t, err)
	assert.Empty(t, entries)

	entries, err = es.ReadFrom(ctx, "agg-2", 0)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestDelete_NonExistentAggregate_IsIdempotent(t *testing.T) {
	es := newTestEventStore(t)

	err := es.Delete(context.Background(), "does-not-exist")
	assert.NoError(t, err)
}

func TestEventStore_Append_DuplicateVersionReturnsPipelineFailed(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte(`{"a":1}`)))

	err := s.Append(ctx, "agg-1", 1, []byte(`{"a":2}`))
	require.Error(t, err)
	assert.ErrorIs(t, err, models.ErrPipelineFailed)
	assert.Contains(t, err.Error(), "version conflict")
}

func TestEventStore_Append_StorageFailureIsNotReportedAsConflict(t *testing.T) {
	s := newTestEventStore(t)
	require.NoError(t, s.Close())

	err := s.Append(context.Background(), "agg-1", 1, []byte(`{"a":1}`))
	require.Error(t, err)
	assert.NotErrorIs(t, err, models.ErrPipelineFailed)
	assert.NotContains(t, err.Error(), "version conflict")
}

func TestEventStore_ListAggregateIDs_StripsPrefixAndDedupes(t *testing.T) {
	store := newTestEventStore(t)
	ctx := context.Background()

	// asynx writes event rows as "events:"+id and snapshot rows as "snapshots:"+id
	// into the same table. Simulate both for two aggregates.
	require.NoError(t, store.Append(ctx, "events:github.com/u/a@v1", 1, []byte(`{}`)))
	require.NoError(t, store.Append(ctx, "events:github.com/u/a@v1", 2, []byte(`{}`)))
	require.NoError(t, store.Append(ctx, "snapshots:github.com/u/a@v1", 2, []byte(`{}`)))
	require.NoError(t, store.Append(ctx, "events:github.com/u/b@main", 1, []byte(`{}`)))

	ids, err := store.ListAggregateIDs(ctx)
	require.NoError(t, err)

	// Sort for stable comparison
	var sortedIDs []string
	sortedIDs = append(sortedIDs, ids...)
	for i := 0; i < len(sortedIDs)-1; i++ {
		for j := i + 1; j < len(sortedIDs); j++ {
			if sortedIDs[i] > sortedIDs[j] {
				sortedIDs[i], sortedIDs[j] = sortedIDs[j], sortedIDs[i]
			}
		}
	}

	assert.Equal(t, []string{"github.com/u/a@v1", "github.com/u/b@main"}, sortedIDs)
}

func TestEventStore_ListAggregateIDs_EmptyReturnsEmpty(t *testing.T) {
	store := newTestEventStore(t)

	ids, err := store.ListAggregateIDs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, ids)
}
