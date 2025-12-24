package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestArrowCreatedPayload struct {
	ArrowName string `json:"arrow_name"`
	Version   string `json:"version"`
}

type TestArrowDeletedPayload struct {
	ArrowName string `json:"arrow_name"`
	Reason    string `json:"reason"`
}

func setupTestStore(t *testing.T) (*SQLiteStore, func()) {
	ctx := context.Background()

	dbName := "test_events_" + uuid.New().String()
	store, err := NewSQLiteStore(ctx, dbName)
	require.NoError(t, err)
	require.NotNil(t, store)

	cleanup := func() {
		store.Close()
	}

	return store, cleanup
}

func TestNewSQLiteStore(t *testing.T) {
	ctx := context.Background()
	dbName := "test_new_store_" + uuid.New().String()

	store, err := NewSQLiteStore(ctx, dbName)
	require.NoError(t, err)
	assert.NotNil(t, store)

	defer store.Close()
}

func TestSQLiteStore_AppendAndGet(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	aggregateID := uuid.New().String()

	payload := &TestArrowCreatedPayload{
		ArrowName: "test-arrow",
		Version:   "1.0.0",
	}

	evt, err := event.NewEvent[TestArrowCreatedPayload]().
		WithAggregateID(aggregateID).
		WithAggregateType("Arrow").
		WithAggregateVersion(1).
		WithEventType("arrow.created").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	var anyPayload any = payload
	anyEvent := &event.Event[any]{
		EventID:          evt.EventID,
		AggregateID:      evt.AggregateID,
		AggregateType:    evt.AggregateType,
		AggregateVersion: evt.AggregateVersion,
		EventType:        evt.EventType,
		EventVersion:     evt.EventVersion,
		ParentID:         evt.ParentID,
		Metadata:         evt.Metadata,
		Payload:          &anyPayload,
		Timestamp:        evt.Timestamp,
	}

	err = store.Append(ctx, anyEvent)
	require.NoError(t, err)

	events, err := GetByAggregate[TestArrowCreatedPayload](store, ctx, aggregateID)
	require.NoError(t, err)
	assert.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.Equal(t, payload.ArrowName, retrievedEvent.Payload.ArrowName)
	assert.Equal(t, payload.Version, retrievedEvent.Payload.Version)
	assert.Equal(t, aggregateID, retrievedEvent.AggregateID)
	assert.Equal(t, "arrow.created", retrievedEvent.EventType)
}

func TestSQLiteStore_MultipleEvents(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	aggregateID := uuid.New().String()

	createdPayload := &TestArrowCreatedPayload{
		ArrowName: "test-arrow",
		Version:   "1.0.0",
	}
	createdEvt, err := event.NewEvent[TestArrowCreatedPayload]().
		WithAggregateID(aggregateID).
		WithAggregateType("Arrow").
		WithAggregateVersion(1).
		WithEventType("arrow.created").
		WithEventVersion("v1").
		WithPayload(createdPayload).
		Build()
	require.NoError(t, err)

	var anyCreatedPayload any = createdPayload
	anyCreatedEvent := &event.Event[any]{
		EventID:          createdEvt.EventID,
		AggregateID:      createdEvt.AggregateID,
		AggregateType:    createdEvt.AggregateType,
		AggregateVersion: createdEvt.AggregateVersion,
		EventType:        createdEvt.EventType,
		EventVersion:     createdEvt.EventVersion,
		ParentID:         createdEvt.ParentID,
		Metadata:         createdEvt.Metadata,
		Payload:          &anyCreatedPayload,
		Timestamp:        createdEvt.Timestamp,
	}

	err = store.Append(ctx, anyCreatedEvent)
	require.NoError(t, err)

	deletedPayload := &TestArrowDeletedPayload{
		ArrowName: "test-arrow",
		Reason:    "testing",
	}
	deletedEvt, err := event.NewEvent[TestArrowDeletedPayload]().
		WithAggregateID(aggregateID).
		WithAggregateType("Arrow").
		WithAggregateVersion(2).
		WithEventType("arrow.deleted").
		WithEventVersion("v1").
		WithPayload(deletedPayload).
		Build()
	require.NoError(t, err)

	var anyDeletedPayload any = deletedPayload
	anyDeletedEvent := &event.Event[any]{
		EventID:          deletedEvt.EventID,
		AggregateID:      deletedEvt.AggregateID,
		AggregateType:    deletedEvt.AggregateType,
		AggregateVersion: deletedEvt.AggregateVersion,
		EventType:        deletedEvt.EventType,
		EventVersion:     deletedEvt.EventVersion,
		ParentID:         deletedEvt.ParentID,
		Metadata:         deletedEvt.Metadata,
		Payload:          &anyDeletedPayload,
		Timestamp:        deletedEvt.Timestamp,
	}

	err = store.Append(ctx, anyDeletedEvent)
	require.NoError(t, err)

	events, err := store.GetByAggregate(ctx, aggregateID)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestSQLiteStore_AggregateExists(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	aggregateID := uuid.New().String()

	exists, err := store.AggregateExists(ctx, aggregateID)
	require.NoError(t, err)
	assert.False(t, exists)

	payload := &TestArrowCreatedPayload{
		ArrowName: "test-arrow",
		Version:   "1.0.0",
	}
	evt, err := event.NewEvent[TestArrowCreatedPayload]().
		WithAggregateID(aggregateID).
		WithAggregateType("Arrow").
		WithAggregateVersion(1).
		WithEventType("arrow.created").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	var anyPayload any = payload
	anyEvent := &event.Event[any]{
		EventID:          evt.EventID,
		AggregateID:      evt.AggregateID,
		AggregateType:    evt.AggregateType,
		AggregateVersion: evt.AggregateVersion,
		EventType:        evt.EventType,
		EventVersion:     evt.EventVersion,
		ParentID:         evt.ParentID,
		Metadata:         evt.Metadata,
		Payload:          &anyPayload,
		Timestamp:        evt.Timestamp,
	}

	err = store.Append(ctx, anyEvent)
	require.NoError(t, err)

	exists, err = store.AggregateExists(ctx, aggregateID)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSQLiteStore_HasEventType(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	aggregateID := uuid.New().String()

	has, err := store.HasEventType(ctx, aggregateID, "arrow.created")
	require.NoError(t, err)
	assert.False(t, has)

	payload := &TestArrowCreatedPayload{
		ArrowName: "test-arrow",
		Version:   "1.0.0",
	}
	evt, err := event.NewEvent[TestArrowCreatedPayload]().
		WithAggregateID(aggregateID).
		WithAggregateType("Arrow").
		WithAggregateVersion(1).
		WithEventType("arrow.created").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	var anyPayload any = payload
	anyEvent := &event.Event[any]{
		EventID:          evt.EventID,
		AggregateID:      evt.AggregateID,
		AggregateType:    evt.AggregateType,
		AggregateVersion: evt.AggregateVersion,
		EventType:        evt.EventType,
		EventVersion:     evt.EventVersion,
		ParentID:         evt.ParentID,
		Metadata:         evt.Metadata,
		Payload:          &anyPayload,
		Timestamp:        evt.Timestamp,
	}

	err = store.Append(ctx, anyEvent)
	require.NoError(t, err)

	has, err = store.HasEventType(ctx, aggregateID, "arrow.created")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.HasEventType(ctx, aggregateID, "arrow.deleted")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestSQLiteStore_EventWithMetadata(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	aggregateID := uuid.New().String()

	payload := &TestArrowCreatedPayload{
		ArrowName: "test-arrow",
		Version:   "1.0.0",
	}

	metadata := map[string]string{
		"user": "admin",
		"ip":   "192.168.1.1",
	}

	evt, err := event.NewEvent[TestArrowCreatedPayload]().
		WithAggregateID(aggregateID).
		WithAggregateType("Arrow").
		WithAggregateVersion(1).
		WithEventType("arrow.created").
		WithEventVersion("v1").
		WithMetadata(metadata).
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	var anyPayload any = payload
	anyEvent := &event.Event[any]{
		EventID:          evt.EventID,
		AggregateID:      evt.AggregateID,
		AggregateType:    evt.AggregateType,
		AggregateVersion: evt.AggregateVersion,
		EventType:        evt.EventType,
		EventVersion:     evt.EventVersion,
		ParentID:         evt.ParentID,
		Metadata:         evt.Metadata,
		Payload:          &anyPayload,
		Timestamp:        evt.Timestamp,
	}

	err = store.Append(ctx, anyEvent)
	require.NoError(t, err)

	events, err := GetByAggregate[TestArrowCreatedPayload](store, ctx, aggregateID)
	require.NoError(t, err)
	assert.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.Equal(t, "admin", retrievedEvent.Metadata["user"])
	assert.Equal(t, "192.168.1.1", retrievedEvent.Metadata["ip"])
}
