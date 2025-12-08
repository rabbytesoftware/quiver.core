package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/memory"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/sqlite"
	"github.com/stretchr/testify/assert"
)

type TestEvent struct {
	eventsourcing.BaseEvent
	Data string `json:"data"`
}

func TestSQLiteEventStore(t *testing.T) {
	ctx := context.Background()

	dbName := "test_sqlite_store_unique_1"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_123", "test", "test.event"),
		Data:      "test data",
	}

	err := store.Append(ctx, event)
	assert.NoError(t, err)

	events, err := store.GetByAggregate(ctx, "agg_123")
	assert.NoError(t, err)
	assert.Len(t, events, 1)

	retrieved := events[0].(*TestEvent)
	assert.Equal(t, "test data", retrieved.Data)

	count, err := store.CountByAggregate(ctx, "agg_123")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	byType, err := store.GetByType(ctx, "test.event")
	assert.NoError(t, err)
	assert.Len(t, byType, 1)
}

func TestSQLiteEventStoreViaBuilder(t *testing.T) {
	ctx := context.Background()

	dbName := "test_builder_store_unique_2"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)
	bus := memory.NewEventBus()

	ev := eventsourcing.NewBuilder[eventsourcing.Event]().
		WithStore(store).
		WithBus(bus).
		WithEventTypes([]eventsourcing.EventTypeRegistration[eventsourcing.Event]{
			{EventType: "test.event", Factory: func() eventsourcing.Event { return &TestEvent{} }},
		}).
		Build()

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_456", "test", "test.event"),
		Data:      "builder test",
	}

	err := ev.Emit(ctx, event)
	assert.NoError(t, err)

	stream := ev.GetEvents(ctx, "agg_456")
	assert.Equal(t, 1, stream.Count())
}

func TestSQLiteEventStore_GetByAggregateWithDeserializationError(t *testing.T) {
	ctx := context.Background()

	dbName := "test_deserialize_error"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_789", "test", "test.event"),
		Data:      "test data",
	}

	err := store.Append(ctx, event)
	assert.NoError(t, err)

	registryWithoutEvent := eventsourcing.NewRegistry[eventsourcing.Event]()
	storeWithBadRegistry := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registryWithoutEvent)

	events, err := storeWithBadRegistry.GetByAggregate(ctx, "agg_789")
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestSQLiteEventStore_GetByTypeWithDeserializationError(t *testing.T) {
	ctx := context.Background()

	dbName := "test_type_deserialize_error"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_890", "test", "test.event"),
		Data:      "test data",
	}

	err := store.Append(ctx, event)
	assert.NoError(t, err)

	registryWithoutEvent := eventsourcing.NewRegistry[eventsourcing.Event]()
	storeWithBadRegistry := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registryWithoutEvent)

	events, err := storeWithBadRegistry.GetByType(ctx, "test.event")
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestSQLiteEventStore_MultipleEventsAndCounting(t *testing.T) {
	ctx := context.Background()

	dbName := "test_multiple_events"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	for i := 0; i < 5; i++ {
		event := &TestEvent{
			BaseEvent: eventsourcing.NewBaseEvent("agg_multi", "test", "test.event"),
			Data:      "test data",
		}
		err := store.Append(ctx, event)
		assert.NoError(t, err)
	}

	count, err := store.CountByAggregate(ctx, "agg_multi")
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)

	events, err := store.GetByAggregate(ctx, "agg_multi")
	assert.NoError(t, err)
	assert.Len(t, events, 5)

	byType, err := store.GetByType(ctx, "test.event")
	assert.NoError(t, err)
	assert.Len(t, byType, 5)
}

func TestSQLiteEventStore_EmptyAggregate(t *testing.T) {
	ctx := context.Background()

	dbName := "test_empty_aggregate"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	events, err := store.GetByAggregate(ctx, "nonexistent_aggregate")
	assert.NoError(t, err)
	assert.Empty(t, events)

	count, err := store.CountByAggregate(ctx, "nonexistent_aggregate")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	byType, err := store.GetByType(ctx, "nonexistent.type")
	assert.NoError(t, err)
	assert.Empty(t, byType)
}

func TestSQLiteEventStore_MultipleAggregates(t *testing.T) {
	ctx := context.Background()

	dbName := "test_multi_agg"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	for i := 0; i < 3; i++ {
		event := &TestEvent{
			BaseEvent: eventsourcing.NewBaseEvent("agg_a", "test", "test.event"),
			Data:      "data a",
		}
		store.Append(ctx, event)
	}

	for i := 0; i < 2; i++ {
		event := &TestEvent{
			BaseEvent: eventsourcing.NewBaseEvent("agg_b", "test", "test.event"),
			Data:      "data b",
		}
		store.Append(ctx, event)
	}

	countA, err := store.CountByAggregate(ctx, "agg_a")
	assert.NoError(t, err)
	assert.Equal(t, int64(3), countA)

	countB, err := store.CountByAggregate(ctx, "agg_b")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), countB)

	eventsA, err := store.GetByAggregate(ctx, "agg_a")
	assert.NoError(t, err)
	assert.Len(t, eventsA, 3)

	eventsB, err := store.GetByAggregate(ctx, "agg_b")
	assert.NoError(t, err)
	assert.Len(t, eventsB, 2)

	allByType, err := store.GetByType(ctx, "test.event")
	assert.NoError(t, err)
	assert.Len(t, allByType, 5)
}

func TestSQLiteEventStore_ContextCancellation(t *testing.T) {
	dbName := "test_context_cancel"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](context.Background(), dbName, registry)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_cancel", "test", "test.event"),
		Data:      "cancelled",
	}

	err := store.Append(ctx, event)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to create event record")
	}
}

func TestSQLiteEventStore_OrderedRetrieval(t *testing.T) {
	ctx := context.Background()

	dbName := "test_ordered"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	for i := 0; i < 3; i++ {
		event := &TestEvent{
			BaseEvent: eventsourcing.NewBaseEvent("agg_ordered", "test", "test.event"),
			Data:      string(rune('A' + i)),
		}
		store.Append(ctx, event)
	}

	events, err := store.GetByAggregate(ctx, "agg_ordered")
	assert.NoError(t, err)
	assert.Len(t, events, 3)

	testEvents := make([]*TestEvent, 0)
	for _, e := range events {
		testEvents = append(testEvents, e.(*TestEvent))
	}

	assert.Equal(t, "A", testEvents[0].Data)
	assert.Equal(t, "B", testEvents[1].Data)
	assert.Equal(t, "C", testEvents[2].Data)
}

func TestSQLiteEventStore_ComplexScenario(t *testing.T) {
	ctx := context.Background()

	dbName := "test_complex_scenario"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("type.a", func() eventsourcing.Event {
		return &TestEvent{}
	})
	registry.Register("type.b", func() eventsourcing.Event {
		return &TestEvent{}
	})
	registry.Register("type.c", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	event1 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a"),
		Data:      "event 1",
	}
	event2 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.b"),
		Data:      "event 2",
	}
	event3 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_2", "test", "type.a"),
		Data:      "event 3",
	}
	event4 := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_2", "test", "type.c"),
		Data:      "event 4",
	}

	store.Append(ctx, event1)
	store.Append(ctx, event2)
	store.Append(ctx, event3)
	store.Append(ctx, event4)

	eventsAgg1, _ := store.GetByAggregate(ctx, "agg_1")
	assert.Len(t, eventsAgg1, 2)

	eventsAgg2, _ := store.GetByAggregate(ctx, "agg_2")
	assert.Len(t, eventsAgg2, 2)

	eventsTypeA, _ := store.GetByType(ctx, "type.a")
	assert.Len(t, eventsTypeA, 2)

	eventsTypeB, _ := store.GetByType(ctx, "type.b")
	assert.Len(t, eventsTypeB, 1)

	eventsTypeC, _ := store.GetByType(ctx, "type.c")
	assert.Len(t, eventsTypeC, 1)

	countAgg1, _ := store.CountByAggregate(ctx, "agg_1")
	assert.Equal(t, int64(2), countAgg1)

	countAgg2, _ := store.CountByAggregate(ctx, "agg_2")
	assert.Equal(t, int64(2), countAgg2)
}

func TestSQLiteEventStore_LargePayload(t *testing.T) {
	ctx := context.Background()

	dbName := "test_large_payload"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &TestEvent{}
	})

	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	largeData := make([]byte, 10000)
	for i := range largeData {
		largeData[i] = byte('A' + (i % 26))
	}

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_large", "test", "test.event"),
		Data:      string(largeData),
	}

	err := store.Append(ctx, event)
	assert.NoError(t, err)

	events, err := store.GetByAggregate(ctx, "agg_large")
	assert.NoError(t, err)
	assert.Len(t, events, 1)

	retrieved := events[0].(*TestEvent)
	assert.Equal(t, string(largeData), retrieved.Data)
}
