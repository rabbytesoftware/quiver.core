package domain

import (
	"context"
)

type Command interface {
	GetAggregateID() string

	Validate(
		ctx context.Context, 
		es EventSourcingValidator,
	) error

	ToRequestedEvent() Event
}

type EventSourcingValidator interface {
	AggregateExists(
		ctx context.Context, 
		aggregateID string,
	) (bool, error)

	HasEventType(
		ctx context.Context, 
		aggregateID string, 
		eventType string,
	) (bool, error)

	GetEvents(
		ctx context.Context, 
		aggregateID string,
	) (EventStream, error)
}

type EventStream interface {
	HasEventType(
		eventType string,
	) bool
	
	GetCurrentState() interface{}	
}
