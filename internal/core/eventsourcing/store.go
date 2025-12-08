package eventsourcing

import "context"

type EventStore[T Event] interface {
	Append(ctx context.Context, event T) error
	GetByAggregate(ctx context.Context, aggregateID string) ([]T, error)
	GetByType(ctx context.Context, eventType string) ([]T, error)
	CountByAggregate(ctx context.Context, aggregateID string) (int64, error)
}
