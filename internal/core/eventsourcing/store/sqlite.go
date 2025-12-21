package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/database"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type Event struct {
	EventID          string    `gorm:"column:event_id;primaryKey;type:varchar(36)"`
	AggregateID      string    `gorm:"column:aggregate_id;index;type:varchar(255)"`
	AggregateType    string    `gorm:"column:aggregate_type;index;type:varchar(255)"`
	EventType        string    `gorm:"column:event_type;index;type:varchar(255)"`
	AggregateVersion int64     `gorm:"column:aggregate_version;index"`
	CorrelationID    string    `gorm:"column:correlation_id;index;type:varchar(36)"`
	ParentID         *string   `gorm:"column:parent_id;index;type:varchar(36)"`
	Timestamp        string    `gorm:"column:timestamp;index;type:varchar(50)"`
	Metadata         string    `gorm:"column:metadata;type:text"`
	EventVersion     string    `gorm:"column:event_version;type:varchar(50)"`
	Payload          string    `gorm:"column:payload;type:text"`
}

func (Event) TableName() string {
	return "events"
}

type SQLiteStore struct {
	repo interface {
		Create(ctx context.Context, entity *Event) (*Event, error)
		Where(ctx context.Context, query string, args ...interface{}) ([]*Event, error)
		Close() error
	}
}

func NewSQLiteStore(ctx context.Context, name string) (*SQLiteStore, error) {
	repo, err := database.NewDatabase[Event](ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}

	return &SQLiteStore{
		repo: repo,
	}, nil
}

func (s *SQLiteStore) Append(ctx context.Context, event domain.Event) error {
	payloadJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event payload: %w", err)
	}

	metadataJSON, err := json.Marshal(event.GetMetadata())
	if err != nil {
		return fmt.Errorf("failed to serialize event metadata: %w", err)
	}

	parentID := event.GetParentID()
	var parentIDStr *string
	if parentID != nil {
		parentIDStr = parentID
	}

	eventRecord := Event{
		EventID:          event.GetEventID(),
		AggregateID:      event.GetAggregateID(),
		AggregateType:    event.GetAggregateType(),
		EventType:        event.GetEventType(),
		AggregateVersion: event.GetAggregateVersion(),
		CorrelationID:    event.GetCorrelationID(),
		ParentID:         parentIDStr,
		Timestamp:        event.GetTimestamp().Format("2006-01-02T15:04:05.000Z07:00"),
		Metadata:         string(metadataJSON),
		EventVersion:     "1.0",
		Payload:          string(payloadJSON),
	}

	_, err = s.repo.Create(ctx, &eventRecord)
	if err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetByAggregate(ctx context.Context, aggregateID string) ([]domain.Event, error) {
	events, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by aggregate: %w", err)
	}

	result := make([]domain.Event, 0, len(events))
	for _, eventRecord := range events {
		event, err := s.deserializeEvent(eventRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize event: %w", err)
		}
		result = append(result, event)
	}

	return result, nil
}

func (s *SQLiteStore) GetNextVersion(ctx context.Context, aggregateID string) (int64, error) {
	events, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return 0, fmt.Errorf("failed to get events for version calculation: %w", err)
	}

	maxVersion := int64(0)
	for _, event := range events {
		if event.AggregateVersion > maxVersion {
			maxVersion = event.AggregateVersion
		}
	}

	return maxVersion + 1, nil
}

func (s *SQLiteStore) AggregateExists(ctx context.Context, aggregateID string) (bool, error) {
	events, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return false, fmt.Errorf("failed to check aggregate existence: %w", err)
	}

	return len(events) > 0, nil
}

func (s *SQLiteStore) deserializeEvent(eventRecord *Event) (domain.Event, error) {
	return nil, fmt.Errorf("deserialization requires event registry - not implemented in store")
}

func (s *SQLiteStore) Close() error {
	return s.repo.Close()
}

