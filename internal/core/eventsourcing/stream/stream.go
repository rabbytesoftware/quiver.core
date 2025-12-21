package stream

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

var _ domain.EventStream = (*EventStream)(nil)

type EventStream struct {
	events []domain.Event
}

func NewEventStream(events []domain.Event) *EventStream {
	return &EventStream{
		events: events,
	}
}

func (s *EventStream) HasSucceededEvent(eventTypePrefix string) bool {
	succeededType := eventTypePrefix + ".Succeeded"
	for _, event := range s.events {
		if event.GetEventType() == succeededType {
			return true
		}
	}
	return false
}

func (s *EventStream) GetCurrentState() interface{} {
	return nil
}

func (s *EventStream) IsExecuting() bool {
	for i := len(s.events) - 1; i >= 0; i-- {
		eventType := s.events[i].GetEventType()
		if contains(eventType, "ExecuteMethod.Started") {
			for j := i + 1; j < len(s.events); j++ {
				nextType := s.events[j].GetEventType()
				if contains(nextType, "ExecuteMethod.Succeeded") || contains(nextType, "ExecuteMethod.Failed") {
					return false
				}
			}
			return true
		}
	}
	return false
}

func (s *EventStream) GetLastExecutionFor(methodName string) interface{} {
	for i := len(s.events) - 1; i >= 0; i-- {
		eventType := s.events[i].GetEventType()
		if contains(eventType, "ExecuteMethod.Requested") && contains(eventType, methodName) {
			return s.events[i]
		}
	}
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

