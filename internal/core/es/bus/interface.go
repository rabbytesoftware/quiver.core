package bus

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

type EventBus interface {
	Publish(
		ctx context.Context,
		event *event.Event[any],
	) error
	Subscribe(
		eventTypeOrPattern string,
		handler EventHandler,
	) error
}

type EventHandler func(ctx context.Context, event *event.Event[any]) error
