package eventsourcing

import (
	"context"
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/bus"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/enricher"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/idempotency"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/registry"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/store"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/stream"
)

const (
	EventNamespace = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
)

type EventSourcing struct {
	store            store.EventStore
	bus              bus.EventBus
	idempotencyStore *idempotency.Store
	registry         *registry.Registry
	enricher         *enricher.Enricher
}

func (es *EventSourcing) ExecuteCommand(
	ctx context.Context, 
	command domain.Command,
) error {
	if err := command.Validate(ctx, es); err != nil {
		return fmt.Errorf("command validation failed: %w", err)
	}

	event := command.ToRequestedEvent()

	if event.ShouldCheckIdempotency() {
		exists, err := es.idempotencyStore.CheckAndSkipIfExists(
			ctx,
			EventNamespace,
			event.GetAggregateID(),
			event.GetAggregateType(),
			event.GetEventType(),
			event.GetMetadata(),
		)
		if err != nil {
			return fmt.Errorf("failed to check idempotency: %w", err)
		}

		if exists {
			return nil
		}
	}

	if err := es.enricher.EnrichEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to enrich event: %w", err)
	}

	if err := es.store.Append(ctx, event); err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	if err := es.bus.Publish(ctx, event); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	if event.ShouldCheckIdempotency() {
		if err := es.idempotencyStore.RecordEvent(
			ctx,
			EventNamespace,
			event.GetAggregateID(),
			event.GetAggregateType(),
			event.GetEventType(),
			event.GetCorrelationID(),
			event.GetMetadata(),
			24*time.Hour,
		); err != nil {
			return fmt.Errorf("failed to store idempotency record: %w", err)
		}
	}

	return nil
}

func (es *EventSourcing) AggregateExists(
	ctx context.Context, 
	aggregateID string,
) (bool, error) {
	return es.store.AggregateExists(ctx, aggregateID)
}

func (es *EventSourcing) HasEventType(
	ctx context.Context, 
	aggregateID string, 
	eventType string,
) (bool, error) {
	events, err := es.store.GetByAggregate(ctx, aggregateID)
	if err != nil {
		return false, err
	}

	stream := stream.NewEventStream(events)
	return stream.HasEventType(eventType), nil
}

func (es *EventSourcing) GetEvents(
	ctx context.Context, 
	aggregateID string,
) (domain.EventStream, error) {
	events, err := es.store.GetByAggregate(ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	return stream.NewEventStream(events), nil
}

func (es *EventSourcing) RegisterEvent(
	event domain.Event,
) {
	es.registry.RegisterEvent(event)
}

func (es *EventSourcing) Subscribe(
	eventTypeOrPattern string, 
	handler bus.EventHandler,
) error {
	return es.bus.Subscribe(eventTypeOrPattern, handler)
}
