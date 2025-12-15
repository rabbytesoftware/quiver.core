package contracts

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type EventBus interface {
	Publish(ctx context.Context, event domain.Event) error
	Subscribe(eventType string, handler Handler) error
}
