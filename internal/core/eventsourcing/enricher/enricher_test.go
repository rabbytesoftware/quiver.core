package enricher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEventStore struct {
	nextVersion int64
	versionErr  error
}

func (m *mockEventStore) Append(ctx context.Context, event domain.Event) error {
	return nil
}

func (m *mockEventStore) GetByAggregate(ctx context.Context, aggregateID string) ([]domain.Event, error) {
	return nil, nil
}

func (m *mockEventStore) GetNextVersion(ctx context.Context, aggregateID string) (int64, error) {
	if m.versionErr != nil {
		return 0, m.versionErr
	}
	return m.nextVersion, nil
}

func (m *mockEventStore) AggregateExists(ctx context.Context, aggregateID string) (bool, error) {
	return false, nil
}

type testEvent struct {
	domain.BaseEvent
	Data string `json:"data"`
}

func (e *testEvent) GetEventType() string {
	return "test.Event"
}

func TestNewEnricher(t *testing.T) {
	mockStore := new(mockEventStore)
	enr := NewEnricher(mockStore)
	assert.NotNil(t, enr)
	assert.Equal(t, mockStore, enr.store)
}

func TestEnricher_EnrichEvent(t *testing.T) {
	mockStore := &mockEventStore{nextVersion: 1}
	enr := NewEnricher(mockStore)
	ctx := context.Background()

	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}

	err := enr.EnrichEvent(ctx, event)
	require.NoError(t, err)

	assert.NotEmpty(t, event.GetEventID())
	assert.NotEmpty(t, event.GetCorrelationID())
	assert.False(t, event.GetTimestamp().IsZero())
	assert.Equal(t, int64(1), event.GetAggregateVersion())
}

func TestEnricher_EnrichEvent_ExistingMetadata(t *testing.T) {
	mockStore := &mockEventStore{nextVersion: 2}
	enr := NewEnricher(mockStore)
	ctx := context.Background()

	existingEventID := uuid.New().String()
	existingCorrelationID := uuid.New().String()
	existingTimestamp := time.Now().Add(-1 * time.Hour)

	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}
	event.SetEventID(existingEventID)
	event.SetCorrelationID(existingCorrelationID)
	event.SetTimestamp(existingTimestamp)

	err := enr.EnrichEvent(ctx, event)
	require.NoError(t, err)

	assert.Equal(t, existingEventID, event.GetEventID())
	assert.Equal(t, existingCorrelationID, event.GetCorrelationID())
	assert.Equal(t, existingTimestamp, event.GetTimestamp())
	assert.Equal(t, int64(2), event.GetAggregateVersion())
}

func TestEnricher_EnrichEvent_StoreError(t *testing.T) {
	mockStore := &mockEventStore{versionErr: errors.New("store error")}
	enr := NewEnricher(mockStore)
	ctx := context.Background()

	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}

	err := enr.EnrichEvent(ctx, event)
	assert.Error(t, err)
}
