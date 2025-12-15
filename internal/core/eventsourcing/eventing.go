package eventsourcing

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/contracts"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	esinternal "github.com/rabbytesoftware/quiver/internal/core/eventsourcing/internal"
)

type EventSourcing struct {
	store    contracts.EventStore
	bus      contracts.EventBus
	registry *esinternal.Registry
}

func (e *EventSourcing) Execute(
	ctx context.Context,
	cmd domain.CommandInterface,
) error {
	if err := cmd.Validate(ctx); err != nil {
		return err
	}

	event := cmd.ToEvent()

	if err := e.store.Append(ctx, event); err != nil {
		return err
	}

	return e.bus.Publish(ctx, event)
}

func (e *EventSourcing) Emit(
	ctx context.Context,
	event domain.Event,
) error {
	if err := e.store.Append(ctx, event); err != nil {
		return err
	}

	return e.bus.Publish(ctx, event)
}

func (e *EventSourcing) RegisterEvent(
	event domain.Event,
) {
	e.registry.Register(event)
}

func (e *EventSourcing) GetEvents(
	ctx context.Context,
	aggregateID string,
) (*domain.EventStream, error) {
	events, err := e.store.GetByAggregate(ctx, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}
	return domain.NewEventStream(events), nil
}

func (e *EventSourcing) Subscribe(
	eventType string,
	handler contracts.Handler,
) error {
	return e.bus.Subscribe(
		eventType,
		handler,
	)
}
