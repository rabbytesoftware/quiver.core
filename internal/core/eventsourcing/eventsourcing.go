package eventsourcing

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
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
	store           store.EventStore
	bus             bus.EventBus
	idempotencyStore *idempotency.Store
	registry        *registry.Registry
	enricher        *enricher.Enricher
}

func (es *EventSourcing) ExecuteCommand(ctx context.Context, command domain.Command) error {
	if err := command.Validate(ctx, es); err != nil {
		return fmt.Errorf("command validation failed: %w", err)
	}

	event := command.ToRequestedEvent()

	if event.ShouldCheckIdempotency() {
		key, err := es.generateIdempotencyKey(event)
		if err != nil {
			return fmt.Errorf("failed to generate idempotency key: %w", err)
		}

		exists, err := es.idempotencyStore.Exists(ctx, key)
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
		key, _ := es.generateIdempotencyKey(event)
		record := &idempotency.Record{
			ID:            key,
			EventType:     event.GetEventType(),
			EventPayload:  "",
			CorrelationID: uuid.MustParse(event.GetCorrelationID()),
			Response:      "",
			CreatedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(24 * time.Hour),
		}
		if err := es.idempotencyStore.Set(ctx, record); err != nil {
			return fmt.Errorf("failed to store idempotency record: %w", err)
		}
	}

	return nil
}

func (es *EventSourcing) AggregateExists(ctx context.Context, aggregateID string) (bool, error) {
	return es.store.AggregateExists(ctx, aggregateID)
}

func (es *EventSourcing) HasSucceededEvent(ctx context.Context, aggregateID string, eventTypePrefix string) (bool, error) {
	events, err := es.store.GetByAggregate(ctx, aggregateID)
	if err != nil {
		return false, err
	}

	stream := stream.NewEventStream(events)
	return stream.HasSucceededEvent(eventTypePrefix), nil
}

func (es *EventSourcing) GetEvents(ctx context.Context, aggregateID string) (domain.EventStream, error) {
	events, err := es.store.GetByAggregate(ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	return stream.NewEventStream(events), nil
}

func (es *EventSourcing) RegisterEvent(event domain.Event) {
	es.registry.RegisterEvent(event)
}

func (es *EventSourcing) Subscribe(eventType string, handler bus.EventHandler) error {
	return es.bus.Subscribe(eventType, handler)
}

func (es *EventSourcing) generateIdempotencyKey(event domain.Event) (uuid.UUID, error) {
	canonicalJSON, err := es.serializeToCanonicalJSON(event)
	if err != nil {
		return uuid.Nil, err
	}

	namespaceUUID := uuid.MustParse(EventNamespace)
	return uuid.NewSHA1(namespaceUUID, []byte(canonicalJSON)), nil
}

func (es *EventSourcing) serializeToCanonicalJSON(event domain.Event) (string, error) {
	eventMap := make(map[string]interface{})
	eventMap["aggregate_id"] = event.GetAggregateID()
	eventMap["aggregate_type"] = event.GetAggregateType()
	eventMap["event_type"] = event.GetEventType()

	metadata := event.GetMetadata()
	if len(metadata) > 0 {
		eventMap["metadata"] = metadata
	}

	jsonBytes, err := json.Marshal(eventMap)
	if err != nil {
		return "", err
	}

	var jsonObj map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &jsonObj); err != nil {
		return "", err
	}

	canonicalBytes, err := es.marshalCanonical(jsonObj)
	if err != nil {
		return "", err
	}

	return string(canonicalBytes), nil
}

func (es *EventSourcing) marshalCanonical(obj map[string]interface{}) ([]byte, error) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := "{"
	for i, key := range keys {
		if i > 0 {
			result += ","
		}
		keyJSON, _ := json.Marshal(key)
		result += string(keyJSON) + ":"
		valJSON, err := json.Marshal(obj[key])
		if err != nil {
			return nil, err
		}
		result += string(valJSON)
	}
	result += "}"

	return []byte(result), nil
}

