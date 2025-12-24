package eventsourcing

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/es/bus"
	es "github.com/rabbytesoftware/quiver/internal/core/es/command"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
	"github.com/rabbytesoftware/quiver/internal/core/es/store"
	"github.com/rabbytesoftware/quiver/internal/core/es/stream"
)

type EventSourcing struct {
	store store.EventStore
	bus   bus.EventBus
}

func Execute[T any](
	e *EventSourcing,
	ctx context.Context,
	command es.Command[T],
) error {
	if err := command.Validate(ctx, e); err != nil {
		return fmt.Errorf("command validation failed: %w", err)
	}

	evt, err := command.ToEvent()
	if err != nil {
		return fmt.Errorf("failed to convert command to event: %w", err)
	}

	anyEvent := toAnyEvent(evt)

	if err := e.store.Append(ctx, anyEvent); err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	if err := e.bus.Publish(ctx, anyEvent); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}

func Publish[T any](
	e *EventSourcing,
	ctx context.Context,
	evt *event.Event[T],
) error {
	anyEvent := toAnyEvent(evt)

	if err := e.store.Append(ctx, anyEvent); err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	if err := e.bus.Publish(ctx, anyEvent); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}

func (e *EventSourcing) AggregateExists(
	ctx context.Context,
	aggregateID string,
) (bool, error) {
	return e.store.AggregateExists(ctx, aggregateID)
}

func (e *EventSourcing) HasEventType(
	ctx context.Context,
	aggregateID string,
	eventType string,
) (bool, error) {
	return e.store.HasEventType(ctx, aggregateID, eventType)
}

func GetEvents[T any](
	e *EventSourcing,
	ctx context.Context,
	aggregateID string,
) ([]*event.Event[T], error) {
	return store.GetByAggregate[T](e.store, ctx, aggregateID)
}

func GetEventStream[T any](
	e *EventSourcing,
	ctx context.Context,
	aggregateID string,
) (*stream.EventStream[T], error) {
	typedEvents, err := GetEvents[T](e, ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	return stream.NewEventStream(typedEvents), nil
}

func (e *EventSourcing) Subscribe(
	eventTypeOrPattern string,
	handler bus.EventHandler,
) error {
	return e.bus.Subscribe(eventTypeOrPattern, handler)
}
