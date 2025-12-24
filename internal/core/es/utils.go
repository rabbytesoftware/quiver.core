package eventsourcing

import "github.com/rabbytesoftware/quiver/internal/core/es/event"

func toAnyEvent[T any](evt *event.Event[T]) *event.Event[any] {
	var anyPayload any
	if evt.Payload != nil {
		anyPayload = *evt.Payload
	}

	return &event.Event[any]{
		EventID:          evt.EventID,
		AggregateID:      evt.AggregateID,
		AggregateType:    evt.AggregateType,
		AggregateVersion: evt.AggregateVersion,
		EventType:        evt.EventType,
		EventVersion:     evt.EventVersion,
		ParentID:         evt.ParentID,
		Metadata:         evt.Metadata,
		Payload:          &anyPayload,
		Timestamp:        evt.Timestamp,
	}
}
