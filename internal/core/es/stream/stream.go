package stream

import (
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

type EventStream[T any] struct {
	events []*event.Event[T]
}

func NewEventStream[T any](
	events []*event.Event[T],
) *EventStream[T] {
	return &EventStream[T]{
		events: events,
	}
}

func (s *EventStream[T]) HasEventType(
	eventType string,
) bool {
	for _, evt := range s.events {
		if evt.EventType == eventType {
			return true
		}
	}
	return false
}

func (s *EventStream[T]) GetEvents() []*event.Event[T] {
	return s.events
}

func (s *EventStream[T]) Len() int {
	return len(s.events)
}

func (s *EventStream[T]) GetLatestVersion() int64 {
	if len(s.events) == 0 {
		return 0
	}
	maxVersion := int64(0)
	for _, evt := range s.events {
		if evt.AggregateVersion > maxVersion {
			maxVersion = evt.AggregateVersion
		}
	}
	return maxVersion
}
