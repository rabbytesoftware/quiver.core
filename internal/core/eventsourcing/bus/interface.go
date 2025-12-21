package bus

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type EventBus interface {
	Publish(ctx context.Context, event domain.Event) error
	Subscribe(eventType string, handler EventHandler) error
}

type EventHandler func(ctx context.Context, event domain.Event) error

