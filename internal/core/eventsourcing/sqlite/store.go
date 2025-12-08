package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/database"
	databaseinterface "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
)

type EventRecord struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	AggregateID   string    `gorm:"index" json:"aggregate_id"`
	AggregateType string    `gorm:"index" json:"aggregate_type"`
	EventType     string    `gorm:"index" json:"event_type"`
	Version       int64     `gorm:"index" json:"version"`
	Timestamp     time.Time `gorm:"index" json:"timestamp"`
	DataJSON      string    `gorm:"type:text" json:"data_json"`
}

type SQLiteEventStore[T eventsourcing.Event] struct {
	repo     databaseinterface.RepositoryInterface[EventRecord]
	registry *eventsourcing.Registry[T]
}

func NewEventStore[T eventsourcing.Event](ctx context.Context, name string, registry *eventsourcing.Registry[T]) eventsourcing.EventStore[T] {
	repo, err := database.NewDatabase[EventRecord](ctx, name)
	if err != nil {
		panic(fmt.Sprintf("failed to create event store: %v", err))
	}

	return &SQLiteEventStore[T]{
		repo:     repo,
		registry: registry,
	}
}

func (s *SQLiteEventStore[T]) Append(ctx context.Context, event T) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	record := &EventRecord{
		ID:            event.GetID(),
		AggregateID:   event.GetAggregateID(),
		AggregateType: event.GetAggregateType(),
		EventType:     event.GetEventType(),
		Version:       event.GetVersion(),
		Timestamp:     event.GetTimestamp(),
		DataJSON:      string(data),
	}

	_, err = s.repo.Create(ctx, record)
	if err != nil {
		return fmt.Errorf("failed to create event record: %w", err)
	}

	return nil
}

func (s *SQLiteEventStore[T]) GetByAggregate(ctx context.Context, aggregateID string) ([]T, error) {
	records, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	var events []T
	for _, record := range records {
		event, err := s.registry.Deserialize(record.EventType, []byte(record.DataJSON))
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

func (s *SQLiteEventStore[T]) GetByType(ctx context.Context, eventType string) ([]T, error) {
	records, err := s.repo.Where(ctx, "event_type = ?", eventType)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	var events []T
	for _, record := range records {
		event, err := s.registry.Deserialize(record.EventType, []byte(record.DataJSON))
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

func (s *SQLiteEventStore[T]) CountByAggregate(ctx context.Context, aggregateID string) (int64, error) {
	records, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}

	return int64(len(records)), nil
}
