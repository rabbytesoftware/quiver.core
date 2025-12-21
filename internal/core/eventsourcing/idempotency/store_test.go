package idempotency

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	store := NewStore()
	assert.NotNil(t, store)
	assert.NotNil(t, store.records)
}

func TestStore_Exists_NotExists(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	key := uuid.New()
	exists, err := store.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestStore_Set_Get(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	key := uuid.New()
	record := &Record{
		ID:            key,
		EventType:     "test.Event",
		EventPayload:  "{}",
		CorrelationID:  uuid.New(),
		Response:      "{}",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}

	err := store.Set(ctx, record)
	require.NoError(t, err)

	exists, err := store.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	retrieved, err := store.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, record.ID, retrieved.ID)
	assert.Equal(t, record.EventType, retrieved.EventType)
}

func TestStore_Get_Expired(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	key := uuid.New()
	record := &Record{
		ID:            key,
		EventType:     "test.Event",
		EventPayload:  "{}",
		CorrelationID:  uuid.New(),
		Response:      "{}",
		CreatedAt:     time.Now().Add(-25 * time.Hour),
		ExpiresAt:     time.Now().Add(-1 * time.Hour),
	}

	err := store.Set(ctx, record)
	require.NoError(t, err)

	exists, err := store.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = store.Get(ctx, key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestStore_Get_NotExists(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	key := uuid.New()
	_, err := store.Get(ctx, key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

