package bus

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/es/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryBus(t *testing.T) {
	bus := NewMemoryBus()
	require.NotNil(t, bus)
	assert.NotNil(t, bus.subscribers)
}

func TestMemoryBus_Subscribe_Success(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	called := false
	handler := func(ctx context.Context, evt *event.Event[any]) error {
		called = true
		return nil
	}

	err := bus.Subscribe("test.event", handler)
	assert.NoError(t, err)

	evt := &event.Event[any]{
		EventType: "test.event",
	}

	err = bus.Publish(ctx, evt)
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestMemoryBus_Subscribe_InvalidPattern(t *testing.T) {
	bus := NewMemoryBus()

	handler := func(ctx context.Context, evt *event.Event[any]) error {
		return nil
	}

	err := bus.Subscribe("[invalid(", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
}

func TestMemoryBus_Publish_NoSubscribers(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	evt := &event.Event[any]{
		EventType: "test.event",
	}

	err := bus.Publish(ctx, evt)
	assert.NoError(t, err)
}

func TestMemoryBus_Publish_MultipleSubscribers(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	called1 := false
	called2 := false

	handler1 := func(ctx context.Context, evt *event.Event[any]) error {
		called1 = true
		return nil
	}

	handler2 := func(ctx context.Context, evt *event.Event[any]) error {
		called2 = true
		return nil
	}

	err := bus.Subscribe("test.event", handler1)
	require.NoError(t, err)

	err = bus.Subscribe("test.event", handler2)
	require.NoError(t, err)

	evt := &event.Event[any]{
		EventType: "test.event",
	}

	err = bus.Publish(ctx, evt)
	assert.NoError(t, err)
	assert.True(t, called1)
	assert.True(t, called2)
}

func TestMemoryBus_Publish_PatternMatching(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	callCount := 0

	handler := func(ctx context.Context, evt *event.Event[any]) error {
		callCount++
		return nil
	}

	err := bus.Subscribe("arrow.*", handler)
	require.NoError(t, err)

	evt1 := &event.Event[any]{
		EventType: "arrow.created",
	}
	err = bus.Publish(ctx, evt1)
	assert.NoError(t, err)

	evt2 := &event.Event[any]{
		EventType: "arrow.deleted",
	}
	err = bus.Publish(ctx, evt2)
	assert.NoError(t, err)

	evt3 := &event.Event[any]{
		EventType: "quiver.created",
	}
	err = bus.Publish(ctx, evt3)
	assert.NoError(t, err)

	assert.Equal(t, 2, callCount)
}

func TestMemoryBus_Publish_HandlerError(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	expectedErr := errors.New("handler error")
	handler := func(ctx context.Context, evt *event.Event[any]) error {
		return expectedErr
	}

	err := bus.Subscribe("test.event", handler)
	require.NoError(t, err)

	evt := &event.Event[any]{
		EventType: "test.event",
	}

	err = bus.Publish(ctx, evt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler failed")
}

func TestMemoryBus_Publish_ExactMatch(t *testing.T) {
	bus := NewMemoryBus()
	ctx := context.Background()

	called := false
	handler := func(ctx context.Context, evt *event.Event[any]) error {
		called = true
		return nil
	}

	err := bus.Subscribe("^arrow\\.created$", handler)
	require.NoError(t, err)

	evt1 := &event.Event[any]{
		EventType: "arrow.created",
	}
	err = bus.Publish(ctx, evt1)
	assert.NoError(t, err)
	assert.True(t, called)

	called = false
	evt2 := &event.Event[any]{
		EventType: "arrow.created.v2",
	}
	err = bus.Publish(ctx, evt2)
	assert.NoError(t, err)
	assert.False(t, called)
}
