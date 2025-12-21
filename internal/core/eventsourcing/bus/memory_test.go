package bus

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	domain.BaseEvent
	Data string `json:"data"`
}

func (e *testEvent) GetEventType() string {
	return "test.Event"
}

func TestNewMemoryBus(t *testing.T) {
	bus := NewMemoryBus()
	assert.NotNil(t, bus)
	assert.NotNil(t, bus.subscribers)
}

func TestMemoryBus_Subscribe(t *testing.T) {
	bus := NewMemoryBus()

	handler := func(ctx context.Context, event domain.Event) error {
		return nil
	}

	err := bus.Subscribe("test.Event", handler)
	require.NoError(t, err)

	assert.Len(t, bus.subscribers["test.Event"], 1)
}

func TestMemoryBus_Publish(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	called := false
	handler := func(ctx context.Context, event domain.Event) error {
		called = true
		return nil
	}

	bus.Subscribe("test.Event", handler)

	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}

	err := bus.Publish(ctx, event)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestMemoryBus_Publish_MultipleHandlers(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	callCount := 0
	handler1 := func(ctx context.Context, event domain.Event) error {
		callCount++
		return nil
	}
	handler2 := func(ctx context.Context, event domain.Event) error {
		callCount++
		return nil
	}

	bus.Subscribe("test.Event", handler1)
	bus.Subscribe("test.Event", handler2)

	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}

	err := bus.Publish(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestMemoryBus_Publish_HandlerError(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	handler := func(ctx context.Context, event domain.Event) error {
		return errors.New("handler error")
	}

	bus.Subscribe("test.Event", handler)

	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}

	err := bus.Publish(ctx, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler failed")
}

func TestMemoryBus_Publish_NoSubscribers(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}

	err := bus.Publish(ctx, event)
	assert.NoError(t, err)
}

