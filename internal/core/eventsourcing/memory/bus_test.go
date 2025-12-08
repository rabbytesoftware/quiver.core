package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/memory"
	"github.com/stretchr/testify/assert"
)

type TestEvent struct {
	eventsourcing.BaseEvent
	Data string `json:"data"`
}

func TestNewEventBus(t *testing.T) {
	bus := memory.NewEventBus()
	assert.NotNil(t, bus)
}

func TestEventBus_Subscribe(t *testing.T) {
	bus := memory.NewEventBus()

	err := bus.Subscribe("test.event", func(ctx context.Context, event eventsourcing.Event) error {
		return nil
	})

	assert.NoError(t, err)
}

func TestEventBus_PublishAndReceive(t *testing.T) {
	ctx := context.Background()
	bus := memory.NewEventBus()

	received := make(chan eventsourcing.Event, 1)

	bus.Subscribe("test.event", func(ctx context.Context, event eventsourcing.Event) error {
		received <- event
		return nil
	})

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
		Data:      "test data",
	}

	err := bus.Publish(ctx, event)
	assert.NoError(t, err)

	select {
	case e := <-received:
		assert.Equal(t, "test.event", e.GetEventType())
		assert.Equal(t, "agg_1", e.GetAggregateID())
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received")
	}
}

func TestEventBus_WildcardSubscription(t *testing.T) {
	ctx := context.Background()
	bus := memory.NewEventBus()

	received := make(chan eventsourcing.Event, 1)

	bus.Subscribe("*", func(ctx context.Context, event eventsourcing.Event) error {
		received <- event
		return nil
	})

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "any.event.type"),
		Data:      "wildcard test",
	}

	err := bus.Publish(ctx, event)
	assert.NoError(t, err)

	select {
	case e := <-received:
		assert.Equal(t, "any.event.type", e.GetEventType())
	case <-time.After(1 * time.Second):
		t.Fatal("Wildcard event not received")
	}
}

func TestEventBus_MultipleHandlers(t *testing.T) {
	ctx := context.Background()
	bus := memory.NewEventBus()

	count1 := 0
	count2 := 0

	bus.Subscribe("test.event", func(ctx context.Context, event eventsourcing.Event) error {
		count1++
		return nil
	})

	bus.Subscribe("test.event", func(ctx context.Context, event eventsourcing.Event) error {
		count2++
		return nil
	})

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
		Data:      "multi handler",
	}

	err := bus.Publish(ctx, event)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
}

func TestEventBus_HandlerError(t *testing.T) {
	ctx := context.Background()
	bus := memory.NewEventBus()

	expectedErr := assert.AnError

	bus.Subscribe("test.event", func(ctx context.Context, event eventsourcing.Event) error {
		return expectedErr
	})

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
		Data:      "error test",
	}

	err := bus.Publish(ctx, event)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestEventBus_MultipleSubscriptionsWildcard(t *testing.T) {
	ctx := context.Background()
	bus := memory.NewEventBus()

	specificReceived := make(chan eventsourcing.Event, 1)
	wildcardReceived := make(chan eventsourcing.Event, 1)

	bus.Subscribe("test.event", func(ctx context.Context, event eventsourcing.Event) error {
		specificReceived <- event
		return nil
	})

	bus.Subscribe("*", func(ctx context.Context, event eventsourcing.Event) error {
		wildcardReceived <- event
		return nil
	})

	event := &TestEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "test.event"),
		Data:      "both handlers",
	}

	err := bus.Publish(ctx, event)
	assert.NoError(t, err)

	select {
	case <-specificReceived:
	case <-time.After(1 * time.Second):
		t.Fatal("Specific handler not called")
	}

	select {
	case <-wildcardReceived:
	case <-time.After(1 * time.Second):
		t.Fatal("Wildcard handler not called")
	}
}

