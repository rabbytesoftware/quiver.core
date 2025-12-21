package domain

import (
	"time"
)

type Event interface {
	GetEventID() string
	GetAggregateID() string
	GetAggregateType() string
	GetEventType() string
	GetAggregateVersion() int64
	GetCorrelationID() string
	GetParentID() *string
	GetTimestamp() time.Time
	GetMetadata() map[string]interface{}
	SetAggregateVersion(version int64)
	SetCorrelationID(correlationID string)
	SetEventID(eventID string)
	SetTimestamp(timestamp time.Time)
	ShouldCheckIdempotency() bool
}

type BaseEvent struct {
	EventID          string                 `json:"event_id" gorm:"column:event_id;primaryKey;type:varchar(36)"`
	AggregateID      string                 `json:"aggregate_id" gorm:"column:aggregate_id;index;type:varchar(255)"`
	AggregateType    string                 `json:"aggregate_type" gorm:"column:aggregate_type;index;type:varchar(255)"`
	EventType        string                 `json:"event_type" gorm:"column:event_type;index;type:varchar(255)"`
	AggregateVersion int64                  `json:"aggregate_version" gorm:"column:aggregate_version;index"`
	CorrelationID    string                 `json:"correlation_id" gorm:"column:correlation_id;index;type:varchar(36)"`
	ParentID         *string                `json:"parent_id,omitempty" gorm:"column:parent_id;index;type:varchar(36)"`
	Timestamp        time.Time              `json:"timestamp" gorm:"column:timestamp;index"`
	Metadata         map[string]interface{} `json:"metadata,omitempty" gorm:"column:metadata;type:text"`
	EventVersion     string                 `json:"event_version" gorm:"column:event_version;type:varchar(50)"`
	Payload          string                 `json:"-" gorm:"column:payload;type:text"`
}

func NewBaseEvent(
	aggregateID, aggregateType, eventType string,
) BaseEvent {
	return BaseEvent{
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		EventVersion:  "1.0",
		Metadata:      make(map[string]interface{}),
	}
}

func NewBaseEventWithMetadata(
	aggregateID, aggregateType, eventType string,
	aggregateVersion int64,
	correlationID string,
	parentID *string,
	metadata map[string]interface{},
) BaseEvent {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	return BaseEvent{
		AggregateID:      aggregateID,
		AggregateType:    aggregateType,
		EventType:        eventType,
		AggregateVersion: aggregateVersion,
		CorrelationID:    correlationID,
		ParentID:         parentID,
		Metadata:         metadata,
		EventVersion:     "1.0",
	}
}

func (e *BaseEvent) GetEventID() string {
	return e.EventID
}

func (e *BaseEvent) GetAggregateID() string {
	return e.AggregateID
}

func (e *BaseEvent) GetAggregateType() string {
	return e.AggregateType
}

func (e *BaseEvent) GetEventType() string {
	return e.EventType
}

func (e *BaseEvent) GetAggregateVersion() int64 {
	return e.AggregateVersion
}

func (e *BaseEvent) GetCorrelationID() string {
	return e.CorrelationID
}

func (e *BaseEvent) GetParentID() *string {
	return e.ParentID
}

func (e *BaseEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e *BaseEvent) GetMetadata() map[string]interface{} {
	if e.Metadata == nil {
		return make(map[string]interface{})
	}
	return e.Metadata
}

func (e *BaseEvent) SetAggregateVersion(version int64) {
	e.AggregateVersion = version
}

func (e *BaseEvent) SetCorrelationID(correlationID string) {
	e.CorrelationID = correlationID
}

func (e *BaseEvent) SetEventID(eventID string) {
	e.EventID = eventID
}

func (e *BaseEvent) SetTimestamp(timestamp time.Time) {
	e.Timestamp = timestamp
}

func (e *BaseEvent) ShouldCheckIdempotency() bool {
	return true
}

func (e *BaseEvent) TableName() string {
	return "events"
}
