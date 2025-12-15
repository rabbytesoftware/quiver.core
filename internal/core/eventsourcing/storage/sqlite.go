package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/database"
	databaseinterface "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/contracts"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	esinternal "github.com/rabbytesoftware/quiver/internal/core/eventsourcing/internal"
)

type EventRecord struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	AggregateID   string    `gorm:"index" json:"aggregate_id"`
	AggregateType string    `gorm:"index" json:"aggregate_type"`
	EventType     string    `gorm:"index" json:"event_type"`
	Version       int64     `gorm:"index" json:"version"`
	Timestamp     time.Time `gorm:"index" json:"timestamp"`
	Payload       string    `gorm:"type:text" json:"payload"`
}

type SQLiteEventStore struct {
	repo        databaseinterface.RepositoryInterface[EventRecord]
	deserialize esinternal.DeserializeFunc
}

func NewSQLiteEventStore(
	ctx context.Context,
	name string,
) (contracts.EventStore, error) {
	repo, err := database.NewDatabase[EventRecord](ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}

	return &SQLiteEventStore{
		repo:        repo,
		deserialize: nil,
	}, nil
}

func (s *SQLiteEventStore) SetDeserializer(fn esinternal.DeserializeFunc) {
	s.deserialize = fn
}

func (s *SQLiteEventStore) Append(ctx context.Context, event domain.Event) error {
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
		Payload:       string(data),
	}

	_, err = s.repo.Create(ctx, record)
	if err != nil {
		return fmt.Errorf("failed to create event record: %w", err)
	}

	return nil
}

func (s *SQLiteEventStore) GetByAggregate(ctx context.Context, aggregateID string) ([]domain.Event, error) {
	records, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	var events []domain.Event
	for _, record := range records {
		event, err := s.deserialize(record.EventType, []byte(record.Payload))
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize event %s: %w", record.ID, err)
		}
		events = append(events, event)
	}

	return events, nil
}

func (s *SQLiteEventStore) GetByType(ctx context.Context, eventType string) ([]domain.Event, error) {
	records, err := s.repo.Where(ctx, "event_type = ?", eventType)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	var events []domain.Event
	for _, record := range records {
		event, err := s.deserialize(record.EventType, []byte(record.Payload))
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize event %s: %w", record.ID, err)
		}
		events = append(events, event)
	}

	return events, nil
}

func (s *SQLiteEventStore) CountByAggregate(ctx context.Context, aggregateID string) (int64, error) {
	records, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}

	return int64(len(records)), nil
}
