package command

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

type EventSourcingValidator interface {
	AggregateExists(ctx context.Context, aggregateID string) (bool, error)
	HasEventType(ctx context.Context, aggregateID string, eventType string) (bool, error)
}

type Command[T any] interface {
	Validate(
		ctx context.Context,
		es EventSourcingValidator,
	) error

	ToEvent() (*event.Event[T], error)
}
