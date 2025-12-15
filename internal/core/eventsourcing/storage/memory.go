package storage

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/contracts"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type MemoryEventStore struct {
	events []domain.Event
	mu     sync.RWMutex
}

func NewMemoryEventStore() contracts.EventStore {
	return &MemoryEventStore{
		events: make([]domain.Event, 0),
	}
}

func (s *MemoryEventStore) Append(ctx context.Context, event domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)
	return nil
}

func (s *MemoryEventStore) GetByAggregate(ctx context.Context, aggregateID string) ([]domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []domain.Event
	for _, event := range s.events {
		if event.GetAggregateID() == aggregateID {
			results = append(results, event)
		}
	}

	return results, nil
}

func (s *MemoryEventStore) GetByType(ctx context.Context, eventType string) ([]domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []domain.Event
	for _, event := range s.events {
		if event.GetEventType() == eventType {
			results = append(results, event)
		}
	}

	return results, nil
}

func (s *MemoryEventStore) CountByAggregate(ctx context.Context, aggregateID string) (int64, error) {
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
