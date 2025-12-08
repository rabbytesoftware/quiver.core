package eventsourcing

import "context"

type Handler func(context.Context, Event) error

type EventBus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventType string, handler Handler) error
}
