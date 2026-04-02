package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func newTestEventStore(t *testing.T) models.Store {
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

func TestEventStore_Count_Success(
	t *testing.T,
) {
	s := newTestEventStore(t)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	require.NoError(t, s.Append(ctx, "agg-1", 3, []byte("e3")))

	count, err := s.Count(ctx, "agg-1", 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestEventStore_Count_EmptyStream(
	t *testing.T,
) {
	s := newTestEventStore(t)
	count, err := s.Count(context.Background(), "nonexistent", 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestEventStore_Count_ContextCancelled(
	t *testing.T,
) {
	s := newTestEventStore(t)
	_, err := s.Count(cancelledCtx(), "agg-1", 1)
	assert.Error(t, err)
}
