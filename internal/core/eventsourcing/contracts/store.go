package contracts

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type EventStore interface {
	Append(ctx context.Context, event domain.Event) error
	GetByAggregate(ctx context.Context, aggregateID string) ([]domain.Event, error)
	GetByType(ctx context.Context, eventType string) ([]domain.Event, error)
	CountByAggregate(ctx context.Context, aggregateID string) (int64, error)
}
