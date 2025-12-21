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
		CorrelationID: uuid.New(),
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
		CorrelationID: uuid.New(),
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

func TestStore_CheckAndSkipIfExists_NotExists(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	exists, err := store.CheckAndSkipIfExists(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", nil)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestStore_CheckAndSkipIfExists_Exists(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	err := store.RecordEvent(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", uuid.New().String(), nil, 24*time.Hour)
	require.NoError(t, err)

	exists, err := store.CheckAndSkipIfExists(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", nil)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestStore_RecordEvent(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	correlationID := uuid.New().String()

	err := store.RecordEvent(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", correlationID, nil, 24*time.Hour)
	require.NoError(t, err)

	exists, err := store.CheckAndSkipIfExists(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", nil)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestStore_RecordEvent_WithMetadata(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	correlationID := uuid.New().String()
	metadata := map[string]interface{}{
		"user": "admin",
		"ip":   "192.168.1.1",
	}

	err := store.RecordEvent(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", correlationID, metadata, 24*time.Hour)
	require.NoError(t, err)

	exists, err := store.CheckAndSkipIfExists(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", metadata)
	require.NoError(t, err)
	assert.True(t, exists)

	differentMetadata := map[string]interface{}{
		"user": "admin",
		"ip":   "192.168.1.2",
	}
	exists, err = store.CheckAndSkipIfExists(ctx, namespace, "agg-123", "Arrow", "ArrowCreated", differentMetadata)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestGenerateKey_Deterministic(t *testing.T) {
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	aggregateID := "aggregate-123"
	aggregateType := "Arrow"
	eventType := "ArrowCreated"
	metadata := map[string]interface{}{
		"user": "admin",
		"ip":   "192.168.1.1",
	}

	key1 := generateKey(namespace, aggregateID, aggregateType, eventType, metadata)
	key2 := generateKey(namespace, aggregateID, aggregateType, eventType, metadata)

	assert.Equal(t, key1, key2, "Keys should be deterministic")
}

func TestGenerateKey_DifferentMetadata(t *testing.T) {
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	aggregateID := "aggregate-123"
	aggregateType := "Arrow"
	eventType := "ArrowCreated"

	metadata1 := map[string]interface{}{
		"user": "admin",
		"ip":   "192.168.1.1",
	}

	metadata2 := map[string]interface{}{
		"user": "admin",
		"ip":   "192.168.1.2",
	}

	key1 := generateKey(namespace, aggregateID, aggregateType, eventType, metadata1)
	key2 := generateKey(namespace, aggregateID, aggregateType, eventType, metadata2)

	assert.NotEqual(t, key1, key2, "Keys should differ with different metadata")
}

func TestGenerateKey_MetadataOrderIndependent(t *testing.T) {
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	aggregateID := "aggregate-123"
	aggregateType := "Arrow"
	eventType := "ArrowCreated"

	metadata1 := map[string]interface{}{
		"user": "admin",
		"ip":   "192.168.1.1",
	}

	metadata2 := map[string]interface{}{
		"ip":   "192.168.1.1",
		"user": "admin",
	}

	key1 := generateKey(namespace, aggregateID, aggregateType, eventType, metadata1)
	key2 := generateKey(namespace, aggregateID, aggregateType, eventType, metadata2)

	assert.Equal(t, key1, key2, "Keys should be the same regardless of metadata key order")
}

func TestGenerateKey_EmptyMetadata(t *testing.T) {
	namespace := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	aggregateID := "aggregate-123"
	aggregateType := "Arrow"
	eventType := "ArrowCreated"

	key1 := generateKey(namespace, aggregateID, aggregateType, eventType, nil)
	key2 := generateKey(namespace, aggregateID, aggregateType, eventType, map[string]interface{}{})

	assert.Equal(t, key1, key2, "Keys should be the same for nil and empty metadata")
}
