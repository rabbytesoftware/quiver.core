package memory

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
)

type MemoryEventStore[T eventsourcing.Event] struct {
	events []T
	mu     sync.RWMutex
}

func NewEventStore[T eventsourcing.Event]() eventsourcing.EventStore[T] {
	return &MemoryEventStore[T]{
		events: make([]T, 0),
	}
}

func (s *MemoryEventStore[T]) Append(ctx context.Context, event T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)
	return nil
}

func (s *MemoryEventStore[T]) GetByAggregate(ctx context.Context, aggregateID string) ([]T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []T
	for _, event := range s.events {
		if event.GetAggregateID() == aggregateID {
			results = append(results, event)
		}
	}

	return results, nil
}

func (s *MemoryEventStore[T]) GetByType(ctx context.Context, eventType string) ([]T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []T
	for _, event := range s.events {
		if event.GetEventType() == eventType {
			results = append(results, event)
		}
	}

	return results, nil
}

func (s *MemoryEventStore[T]) CountByAggregate(ctx context.Context, aggregateID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := int64(0)
	for _, event := range s.events {
		if event.GetAggregateID() == aggregateID {
			count++
		}
	}

	return count, nil
}
