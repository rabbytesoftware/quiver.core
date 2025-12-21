package stream

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type eventStream struct {
	events []domain.Event
}

func NewEventStream(
	events []domain.Event,
) domain.EventStream {
	return &eventStream{
		events: events,
	}
}

func (s *eventStream) HasEventType(
	eventType string,
) bool {
	for _, event := range s.events {
		if event.GetEventType() == eventType {
			return true
		}
	}
	return false
}

func (s *eventStream) GetCurrentState() interface{} {
	return nil
}
