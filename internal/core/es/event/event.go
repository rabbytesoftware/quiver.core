package event

import (
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type EventBuilder[T any] interface {
	WithAggregateID(
		aggregateID string,
	) EventBuilder[T]
	WithAggregateType(
		aggregateType string,
	) EventBuilder[T]
	WithAggregateVersion(
		aggregateVersion int64,
	) EventBuilder[T]

	WithEventType(
		eventType string,
	) EventBuilder[T]
	WithEventVersion(
		eventVersion string,
	) EventBuilder[T]
	WithIdempotency(
		idempotency bool,
	) EventBuilder[T]

	WithParentID(
		parentID *string,
	) EventBuilder[T]

	WithMetadata(
		metadata map[string]string,
	) EventBuilder[T]
	WithPayload(
		payload *T,
	) EventBuilder[T]

	Build() (*Event[T], error)
}

type Event[T any] struct {
	EventID string `json:"event_id" validate:"required" gorm:"column:event_id;primaryKey;type:varchar(36)"`

	AggregateID      string `json:"aggregate_id" validate:"required" gorm:"column:aggregate_id;index;type:varchar(255)"`
	AggregateType    string `json:"aggregate_type" validate:"required" gorm:"column:aggregate_type;index;type:varchar(255)"`
	AggregateVersion int64  `json:"aggregate_version" validate:"required" gorm:"column:aggregate_version;index"`

	EventType    string `json:"event_type" validate:"required" gorm:"column:event_type;index;type:varchar(255)"`
	EventVersion string `json:"event_version" validate:"required" gorm:"column:event_version;type:varchar(50)"`
	Idempotency  bool   `json:"idempotency" validate:"required" gorm:"column:idempotency;type:boolean"`

	ParentID *string `json:"parent_id,omitempty" gorm:"column:parent_id;index;type:varchar(36)"`

	Metadata  map[string]string `json:"metadata,omitempty" gorm:"column:metadata;type:jsonb"`
	Payload   *T                `json:"payload" validate:"required" gorm:"column:payload;type:jsonb"`
	Timestamp time.Time         `json:"timestamp" validate:"required" gorm:"column:timestamp;index;type:timestamp"`
}

type eventBuilder[T any] struct {
	event *Event[T]
}

func NewEvent[T any]() EventBuilder[T] {
	return &eventBuilder[T]{
		event: &Event[T]{
			Idempotency: true,
		},
	}
}

func (e *eventBuilder[T]) WithAggregateID(
	aggregateID string,
) EventBuilder[T] {
	e.event.AggregateID = aggregateID
	return e
}

func (e *eventBuilder[T]) WithAggregateType(
	aggregateType string,
) EventBuilder[T] {
	e.event.AggregateType = aggregateType
	return e
}

func (e *eventBuilder[T]) WithAggregateVersion(
	aggregateVersion int64,
) EventBuilder[T] {
	e.event.AggregateVersion = aggregateVersion
	return e
}

func (e *eventBuilder[T]) WithEventType(
	eventType string,
) EventBuilder[T] {
	e.event.EventType = eventType
	return e
}

func (e *eventBuilder[T]) WithEventVersion(
	eventVersion string,
) EventBuilder[T] {
	e.event.EventVersion = eventVersion
	return e
}

func (e *eventBuilder[T]) WithIdempotency(
	idempotency bool,
) EventBuilder[T] {
	e.event.Idempotency = idempotency
	return e
}

func (e *eventBuilder[T]) WithParentID(
	parentID *string,
) EventBuilder[T] {
	e.event.ParentID = parentID
	return e
}

func (e *eventBuilder[T]) WithMetadata(
	metadata map[string]string,
) EventBuilder[T] {
	e.event.Metadata = metadata
	return e
}

func (e *eventBuilder[T]) WithPayload(
	payload *T,
) EventBuilder[T] {
	e.event.Payload = payload
	return e
}

func (e *eventBuilder[T]) Build() (*Event[T], error) {
	e.event.EventID = uuid.New().String()
	e.event.Timestamp = time.Now().UTC()

	validate := validator.New()
	if err := validate.Struct(e.event); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	return e.event, nil
}
