package eventsourcing

type EventStream[T Event] struct {
	events []T
}

func NewEventStream[T Event](events []T) *EventStream[T] {
	return &EventStream[T]{
		events: events,
	}
}

func (s *EventStream[T]) All() []T {
	return s.events
}

func (s *EventStream[T]) Count() int {
	return len(s.events)
}

func (s *EventStream[T]) CountByType(eventType string) int {
	count := 0
	for _, event := range s.events {
		if event.GetEventType() == eventType {
			count++
		}
	}
	return count
}

func FindLast[E any, T Event](stream *EventStream[T], eventType string) *E {
	for i := len(stream.events) - 1; i >= 0; i-- {
		if stream.events[i].GetEventType() == eventType {
			if concrete, ok := any(stream.events[i]).(*E); ok {
				return concrete
			}
		}
	}
	return nil
}

func Filter[E any, T Event](stream *EventStream[T], eventType string) []*E {
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
