package store

import (
	"context"

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
			if payload, ok := (*anyEvt.Payload).(T); ok {
				typedEvt.Payload = &payload
			}
		}
		typedEvents = append(typedEvents, typedEvt)
	}

	return typedEvents, nil
}
