package domain

import (
	"time"

	"github.com/google/uuid"
)

type Event interface {
	GetID() string
	GetAggregateID() string
	GetAggregateType() string
	GetEventType() string
	GetVersion() int64
	GetTimestamp() time.Time
}

type BaseEvent struct {
	ID            string    `json:"id"`
	AggregateID   string    `json:"aggregate_id"`
	AggregateType string    `json:"aggregate_type"`
	EventType     string    `json:"event_type"`
	Version       int64     `json:"version"`
	Timestamp     time.Time `json:"timestamp"`
}

func NewBaseEvent(
	aggregateID, aggregateType, eventType string,
) BaseEvent {
	return BaseEvent{
		ID:            uuid.New().String(),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		Version:       1,
		Timestamp:     time.Now(),
	}
}

func (e *BaseEvent) GetID() string {
	return e.ID
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

func (e *BaseEvent) GetVersion() int64 {
	return e.Version
}

func (e *BaseEvent) GetTimestamp() time.Time {
	return e.Timestamp
}
