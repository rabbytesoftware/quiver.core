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

type arrowEvent struct {
	domain.BaseEvent
	Action string `json:"action"`
}

func (e *arrowEvent) GetEventType() string {
	return "arrow.AddArrow.Succeeded"
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

	assert.Len(t, bus.subscribers, 1)
	assert.Equal(t, "test.Event", bus.subscribers[0].raw)
}

func TestMemoryBus_Subscribe_InvalidRegex(t *testing.T) {
	bus := NewMemoryBus()

	handler := func(ctx context.Context, event domain.Event) error {
		return nil
	}

	err := bus.Subscribe("[invalid", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
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

func TestMemoryBus_Publish_RegexPattern(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	var capturedEvents []string
	handler := func(ctx context.Context, event domain.Event) error {
		capturedEvents = append(capturedEvents, event.GetEventType())
		return nil
	}

	// Subscribe to all arrow events
	err := bus.Subscribe("^arrow\\..*", handler)
	require.NoError(t, err)

	// Publish arrow event - should match
	arrowEvt := &arrowEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "arrow", "arrow.AddArrow.Succeeded"),
		Action:    "add",
	}
	err = bus.Publish(ctx, arrowEvt)
	require.NoError(t, err)

	// Publish test event - should NOT match
	testEvt := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg2", "test", "test.Event"),
		Data:      "test",
	}
	err = bus.Publish(ctx, testEvt)
	require.NoError(t, err)

	assert.Len(t, capturedEvents, 1)
	assert.Equal(t, "arrow.AddArrow.Succeeded", capturedEvents[0])
}

func TestMemoryBus_Publish_MultiplePatterns(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	successCount := 0
	allCount := 0

	successHandler := func(ctx context.Context, event domain.Event) error {
		successCount++
		return nil
	}
	allHandler := func(ctx context.Context, event domain.Event) error {
		allCount++
		return nil
	}

	// Subscribe to all success events
	bus.Subscribe(".*\\.Succeeded$", successHandler)
	// Subscribe to all events
	bus.Subscribe(".*", allHandler)

	event := &arrowEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "arrow", "arrow.AddArrow.Succeeded"),
		Action:    "add",
	}

	err := bus.Publish(ctx, event)
	require.NoError(t, err)

	// Both handlers should have been called
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, allCount)
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

func TestMemoryBus_Publish_ComplexPatterns(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	var capturedEvents []string
	handler := func(ctx context.Context, event domain.Event) error {
		capturedEvents = append(capturedEvents, event.GetEventType())
		return nil
	}

	// Subscribe to arrow Add or Remove Succeeded events
	bus.Subscribe("^arrow\\.(Add|Remove)Arrow\\.Succeeded$", handler)

	// Should match - AddArrow
	addEvent := &domain.BaseEvent{
		AggregateID:   "agg1",
		AggregateType: "arrow",
		EventType:     "arrow.AddArrow.Succeeded",
	}
	bus.Publish(ctx, addEvent)

	// Should match - RemoveArrow
	removeEvent := &domain.BaseEvent{
		AggregateID:   "agg2",
		AggregateType: "arrow",
		EventType:     "arrow.RemoveArrow.Succeeded",
	}
	bus.Publish(ctx, removeEvent)

	// Should NOT match - ExecuteMethod
	execEvent := &domain.BaseEvent{
		AggregateID:   "agg3",
		AggregateType: "arrow",
		EventType:     "arrow.ExecuteMethod.Succeeded",
	}
	bus.Publish(ctx, execEvent)

	assert.Len(t, capturedEvents, 2)
	assert.Contains(t, capturedEvents, "arrow.AddArrow.Succeeded")
	assert.Contains(t, capturedEvents, "arrow.RemoveArrow.Succeeded")
}
