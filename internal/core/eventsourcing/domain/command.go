package domain

import (
	"context"
)

type Command interface {
	GetAggregateID() string
	Validate(ctx context.Context, es EventSourcingValidator) error
	ToRequestedEvent() Event
}

type EventSourcingValidator interface {
	AggregateExists(ctx context.Context, aggregateID string) (bool, error)
	HasSucceededEvent(ctx context.Context, aggregateID string, eventTypePrefix string) (bool, error)
	GetEvents(ctx context.Context, aggregateID string) (EventStream, error)
}

type EventStream interface {
	HasSucceededEvent(eventTypePrefix string) bool
	GetCurrentState() interface{}
	IsExecuting() bool
	GetLastExecutionFor(methodName string) interface{}
}

