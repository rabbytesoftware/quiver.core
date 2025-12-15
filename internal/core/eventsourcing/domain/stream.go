package domain

type EventStream struct {
	events []Event
}

func NewEventStream(
	events []Event,
) *EventStream {
	return &EventStream{events: events}
}

func (s *EventStream) All() []Event {
	return s.events
}

func (s *EventStream) Count() int {
	return len(s.events)
}

func (s *EventStream) CountByType(
	eventType string,
) int {
	count := 0
	for _, event := range s.events {
		if event.GetEventType() == eventType {
			count++
		}
	}
	return count
}

func FindLast[E any](
	stream *EventStream,
	eventType string,
) *E {
	for i := len(stream.events) - 1; i >= 0; i-- {
		if stream.events[i].GetEventType() == eventType {
			if concrete, ok := any(stream.events[i]).(*E); ok {
				return concrete
			}
		}
	}
	return nil
}

func Filter[E any](
	stream *EventStream,
	eventType string,
) []*E {
	var results []*E
	for _, event := range stream.events {
		if event.GetEventType() == eventType {
			if concrete, ok := any(event).(*E); ok {
				results = append(results, concrete)
			}
		}
	}
	return results
}
