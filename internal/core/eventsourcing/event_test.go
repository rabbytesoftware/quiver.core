package eventsourcing_test

import (
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/stretchr/testify/assert"
)

func TestNewBaseEvent(t *testing.T) {
	baseEvent := eventsourcing.NewBaseEvent("agg_123", "test_type", "test.event")

	assert.NotEmpty(t, baseEvent.GetID())
	assert.Equal(t, "agg_123", baseEvent.GetAggregateID())
	assert.Equal(t, "test_type", baseEvent.GetAggregateType())
	assert.Equal(t, "test.event", baseEvent.GetEventType())
	assert.Equal(t, int64(1), baseEvent.GetVersion())
	assert.False(t, baseEvent.GetTimestamp().IsZero())
	assert.WithinDuration(t, time.Now(), baseEvent.GetTimestamp(), 1*time.Second)
}

func TestBaseEvent_AllGetters(t *testing.T) {
	baseEvent := eventsourcing.NewBaseEvent("aggregate_abc", "arrow", "wizard.started")

	id := baseEvent.GetID()
	assert.NotEmpty(t, id)
	assert.Len(t, id, 36)

	assert.Equal(t, "aggregate_abc", baseEvent.GetAggregateID())
	assert.Equal(t, "arrow", baseEvent.GetAggregateType())
	assert.Equal(t, "wizard.started", baseEvent.GetEventType())
	assert.Equal(t, int64(1), baseEvent.GetVersion())

	timestamp := baseEvent.GetTimestamp()
	assert.False(t, timestamp.IsZero())
	assert.True(t, timestamp.Before(time.Now().Add(1*time.Second)))
}

func TestBaseEvent_DifferentAggregates(t *testing.T) {
	event1 := eventsourcing.NewBaseEvent("agg_1", "type_1", "event.type1")
	event2 := eventsourcing.NewBaseEvent("agg_2", "type_2", "event.type2")

	assert.NotEqual(t, event1.GetID(), event2.GetID())
	assert.NotEqual(t, event1.GetAggregateID(), event2.GetAggregateID())
	assert.NotEqual(t, event1.GetAggregateType(), event2.GetAggregateType())
	assert.NotEqual(t, event1.GetEventType(), event2.GetEventType())
}

