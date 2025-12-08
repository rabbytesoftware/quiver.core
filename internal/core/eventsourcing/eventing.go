package eventsourcing

import "context"

type EventSourcing[T Event] struct {
	store    EventStore[T]
	bus      EventBus
	registry *Registry[T]
}

func NewEventSourcing[T Event](store EventStore[T], bus EventBus, registry *Registry[T]) *EventSourcing[T] {
	return &EventSourcing[T]{
		store:    store,
		bus:      bus,
		registry: registry,
	}
}

func (e *EventSourcing[T]) Emit(ctx context.Context, event T) error {
	if err := e.store.Append(ctx, event); err != nil {
		return err
	}

	return e.bus.Publish(ctx, event)
}

func (e *EventSourcing[T]) GetEvents(ctx context.Context, aggregateID string) *EventStream[T] {
	events, _ := e.store.GetByAggregate(ctx, aggregateID)
	return NewEventStream(events)
}

func (e *EventSourcing[T]) Store() EventStore[T] {
	return e.store
}

func (e *EventSourcing[T]) Bus() EventBus {
	return e.bus
}

func (e *EventSourcing[T]) Registry() *Registry[T] {
	return e.registry
}
