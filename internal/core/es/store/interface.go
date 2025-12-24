package store

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

type EventStore interface {
	Append(ctx context.Context, event *event.Event[any]) error
	GetByAggregate(ctx context.Context, aggregateID string) ([]*event.Event[any], error)
	AggregateExists(ctx context.Context, aggregateID string) (bool, error)
	HasEventType(ctx context.Context, aggregateID string, eventType string) (bool, error)
}
