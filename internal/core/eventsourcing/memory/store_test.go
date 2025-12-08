package memory_test

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/memory"
	"github.com/stretchr/testify/assert"
)

func TestNewEventStore(t *testing.T) {
	store := memory.NewEventStore[eventsourcing.Event]()
	assert.NotNil(t, store)
}

func TestMemoryEventStore_Append(t *testing.T) {
	ctx := context.Background()
	store := memory.NewEventStore[eventsourcing.Event]()

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
		Data:      "test",
	}

	err := store.Append(ctx, event)
	assert.NoError(t, err)

	events, _ := store.GetByAggregate(ctx, "agg_1")
	assert.Len(t, events, 1)
}

func TestMemoryEventStore_GetByAggregate(t *testing.T) {
	ctx := context.Background()
	store := memory.NewEventStore[eventsourcing.Event]()

	event1 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
		Data:      "event 1",
	}
	event2 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
		Data:      "event 2",
	}
	event3 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_2", "test", "test.event"),
		Data:      "event 3",
	}

	store.Append(ctx, event1)
	store.Append(ctx, event2)
	store.Append(ctx, event3)

	eventsAgg1, err := store.GetByAggregate(ctx, "agg_1")
	assert.NoError(t, err)
	assert.Len(t, eventsAgg1, 2)

	eventsAgg2, err := store.GetByAggregate(ctx, "agg_2")
	assert.NoError(t, err)
	assert.Len(t, eventsAgg2, 1)

	eventsNonExistent, err := store.GetByAggregate(ctx, "agg_999")
	assert.NoError(t, err)
	assert.Empty(t, eventsNonExistent)
}

func TestMemoryEventStore_GetByType(t *testing.T) {
	ctx := context.Background()
	store := memory.NewEventStore[eventsourcing.Event]()

	event1 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a"),
		Data:      "event 1",
	}
	event2 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_2", "test", "type.b"),
		Data:      "event 2",
	}
	event3 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_3", "test", "type.a"),
		Data:      "event 3",
	}

	store.Append(ctx, event1)
	store.Append(ctx, event2)
	store.Append(ctx, event3)

	typeAEvents, err := store.GetByType(ctx, "type.a")
	assert.NoError(t, err)
	assert.Len(t, typeAEvents, 2)

	typeBEvents, err := store.GetByType(ctx, "type.b")
	assert.NoError(t, err)
	assert.Len(t, typeBEvents, 1)

	typeNonExistent, err := store.GetByType(ctx, "type.nonexistent")
	assert.NoError(t, err)
	assert.Empty(t, typeNonExistent)
}

func TestMemoryEventStore_CountByAggregate(t *testing.T) {
	ctx := context.Background()
	store := memory.NewEventStore[eventsourcing.Event]()

	for i := 0; i < 5; i++ {
		event := &TestEvent{
			BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
			Data:      "event",
		}
		store.Append(ctx, event)
	}

	for i := 0; i < 3; i++ {
		event := &TestEvent{
			BaseEvent: eventsourcing.NewBaseEvent("agg_2", "test", "test.event"),
			Data:      "event",
		}
		store.Append(ctx, event)
	}

	count1, err := store.CountByAggregate(ctx, "agg_1")
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count1)

	count2, err := store.CountByAggregate(ctx, "agg_2")
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count2)

	countNonExistent, err := store.CountByAggregate(ctx, "agg_999")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), countNonExistent)
}

func TestMemoryEventStore_ConcurrentAppend(t *testing.T) {
	ctx := context.Background()
	store := memory.NewEventStore[eventsourcing.Event]()

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(index int) {
			event := &TestEvent{
				BaseEvent: eventsourcing.NewBaseEvent("agg_concurrent", "test", "test.event"),
				Data:      "concurrent",
			}
			store.Append(ctx, event)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	count, err := store.CountByAggregate(ctx, "agg_concurrent")
	assert.NoError(t, err)
	assert.Equal(t, int64(10), count)
}

