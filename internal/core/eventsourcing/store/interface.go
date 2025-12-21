package store

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type EventStore interface {
	Append(ctx context.Context, event domain.Event) error
	GetByAggregate(ctx context.Context, aggregateID string) ([]domain.Event, error)
	GetNextVersion(ctx context.Context, aggregateID string) (int64, error)
	AggregateExists(ctx context.Context, aggregateID string) (bool, error)
}
