package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/database"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

type eventRow struct {
	EventID string `gorm:"column:event_id;primaryKey;type:varchar(36)"`

	AggregateID      string `gorm:"column:aggregate_id;index;type:varchar(255)"`
	AggregateType    string `gorm:"column:aggregate_type;index;type:varchar(255)"`
	AggregateVersion int64  `gorm:"column:aggregate_version;index"`

	EventType    string `gorm:"column:event_type;index;type:varchar(255)"`
	EventVersion string `gorm:"column:event_version;type:varchar(50)"`

	ParentID *string `gorm:"column:parent_id;index;type:varchar(36)"`

	MetadataJSON string    `gorm:"column:metadata;type:text"`
	PayloadJSON  string    `gorm:"column:payload;type:text"`
	Timestamp    time.Time `gorm:"column:timestamp;index;type:timestamp"`
}

func (eventRow) TableName() string {
	return "events"
}

type SQLiteStore struct {
	repo interfaces.RepositoryInterface[eventRow]
}

func NewSQLiteStore(
	ctx context.Context,
	name string,
) (*SQLiteStore, error) {
	repo, err := database.NewDatabase[eventRow](ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}

	return &SQLiteStore{
		repo: repo,
	}, nil
}

func (s *SQLiteStore) Append(
	ctx context.Context,
	evt *event.Event[any],
) error {
	row, err := s.eventToRow(evt)
	if err != nil {
		return fmt.Errorf("failed to convert event to row: %w", err)
	}

	_, err = s.repo.Create(ctx, row)
	if err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetByAggregate(
	ctx context.Context,
	aggregateID string,
) ([]*event.Event[any], error) {
	rows, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by aggregate: %w", err)
	}

	result := make([]*event.Event[any], 0, len(rows))
	for _, row := range rows {
		evt, err := rowToEvent[any](row)
		if err != nil {
			return nil, fmt.Errorf("failed to convert row to event: %w", err)
		}
		result = append(result, evt)
	}

	return result, nil
}

func (s *SQLiteStore) AggregateExists(
	ctx context.Context,
	aggregateID string,
) (bool, error) {
	rows, err := s.repo.Where(ctx, "aggregate_id = ?", aggregateID)
	if err != nil {
		return false, fmt.Errorf("failed to check aggregate existence: %w", err)
	}

	return len(rows) > 0, nil
}

func (s *SQLiteStore) HasEventType(
	ctx context.Context,
	aggregateID string,
	eventType string,
) (bool, error) {
	rows, err := s.repo.Where(ctx, "aggregate_id = ? AND event_type = ?", aggregateID, eventType)
	if err != nil {
		return false, fmt.Errorf("failed to check event type: %w", err)
	}

	return len(rows) > 0, nil
}

func (s *SQLiteStore) eventToRow(evt *event.Event[any]) (*eventRow, error) {
	metadataJSON := "{}"
	if evt.Metadata != nil {
		metadataBytes, err := json.Marshal(evt.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	payloadBytes, err := json.Marshal(evt.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return &eventRow{
		EventID:          evt.EventID,
		AggregateID:      evt.AggregateID,
		AggregateType:    evt.AggregateType,
		AggregateVersion: evt.AggregateVersion,
		EventType:        evt.EventType,
		EventVersion:     evt.EventVersion,
		ParentID:         evt.ParentID,
		MetadataJSON:     metadataJSON,
		PayloadJSON:      string(payloadBytes),
		Timestamp:        evt.Timestamp,
	}, nil
}

func rowToEvent[T any](row *eventRow) (*event.Event[T], error) {
	var metadata map[string]string
	if row.MetadataJSON != "" && row.MetadataJSON != "{}" {
		if err := json.Unmarshal([]byte(row.MetadataJSON), &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	var payload T
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &event.Event[T]{
		EventID:          row.EventID,
		AggregateID:      row.AggregateID,
		AggregateType:    row.AggregateType,
		AggregateVersion: row.AggregateVersion,
		EventType:        row.EventType,
		EventVersion:     row.EventVersion,
		ParentID:         row.ParentID,
		Metadata:         metadata,
		Payload:          &payload,
		Timestamp:        row.Timestamp,
	}, nil
}

func (s *SQLiteStore) Close() error {
	return s.repo.Close()
}
