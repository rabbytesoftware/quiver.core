package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

func GetByAggregate[T any](
	store EventStore,
	ctx context.Context,
	aggregateID string,
) ([]*event.Event[T], error) {
	anyEvents, err := store.GetByAggregate(ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	typedEvents := make([]*event.Event[T], 0, len(anyEvents))
	for _, anyEvt := range anyEvents {
		typedEvt := &event.Event[T]{
			EventID:          anyEvt.EventID,
			AggregateID:      anyEvt.AggregateID,
			AggregateType:    anyEvt.AggregateType,
			AggregateVersion: anyEvt.AggregateVersion,
			EventType:        anyEvt.EventType,
			EventVersion:     anyEvt.EventVersion,
			ParentID:         anyEvt.ParentID,
			Metadata:         anyEvt.Metadata,
			Timestamp:        anyEvt.Timestamp,
		}
		if anyEvt.Payload != nil {
			payloadBytes, err := json.Marshal(*anyEvt.Payload)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal payload: %w", err)
			}

			var typedPayload T
			if err := json.Unmarshal(payloadBytes, &typedPayload); err != nil {
				return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
			}
			typedEvt.Payload = &typedPayload
		}
		typedEvents = append(typedEvents, typedEvt)
	}

	return typedEvents, nil
}
