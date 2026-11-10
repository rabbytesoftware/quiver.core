// Package bind lets repositories register asynx durable subscribers and sync
// projections after the instance is built.
//
// asynx accepts both only on its Builder, and Build already starts
// redelivering what a crash left pending. The app container therefore builds
// each instance with one catch-all durable subscriber and one catch-all sync
// projection that forward to a Binder; repositories add their routes to it
// while the container is still wiring, and Ready releases the deliveries that
// waited meanwhile.
package bind

import (
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"gorm.io/gorm"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
)

// Build finishes the builder with the two catch-all registrations that forward
// to a new Binder, and pairs the resulting instance with it. The events store
// must be the one the builder was given; when it can host read models
// (eventstore.Hosted) the returned Bound exposes its database through Host.
func Build[T any](
	builder *asynx.Builder[T],
	events asynxModels.Store,
) (Bound[T], error) {
	binder := New[T]()
	ax, err := builder.
		WithSubscriber(".*", binder.DispatchDurable).
		WithSyncProjection(".*", binder.DispatchSync).
		Build()
	if err != nil {
		return nil, fmt.Errorf("bind: build: %w", err)
	}

	var db *gorm.DB
	hosted, ok := events.(eventstore.Hosted)
	if ok {
		db = hosted.DB()
	}
	return Wrap(ax, binder, db), nil
}

// Host returns the events database of ax, where its read model can live and
// commit with its events, or nil when ax was not built by Build or its store
// cannot host one.
func Host[T any](
	ax asynx.Asynx[T],
) *gorm.DB {
	bound, ok := ax.(Bound[T])
	if !ok {
		return nil
	}
	return bound.Host()
}

// OnEvent registers h to receive every event matching pattern, which is what
// asynx's Subscribe takes.
//
// On an instance built by Build the delivery is durable: it is retried with
// backoff and redelivered after a restart, so h must be idempotent on
// (AggregateID, Version). On any other asynx.Asynx it is a plain Subscribe
// whose handler failures are logged.
func OnEvent[T any](
	ax asynx.Asynx[T],
	pattern string,
	h asynxModels.DurableHandler[T],
) error {
	bound, ok := ax.(Bound[T])
	if ok {
		return bound.Binder().Durable(pattern, h)
	}
	_, err := ax.Subscribe(pattern, loggedHandler[T]{handler: h}.handle)
	if err != nil {
		return fmt.Errorf("bind: subscribe %s: %w", pattern, err)
	}
	return nil
}

// Projection registers fn as a sync projection for pattern when ax supports
// it and reports whether it did. When it returns false the caller keeps its
// own projection path.
func Projection[T any](
	ax asynx.Asynx[T],
	pattern string,
	fn SyncFn[T],
) (bool, error) {
	bound, ok := ax.(Bound[T])
	if !ok {
		return false, nil
	}
	err := bound.Binder().Sync(pattern, fn)
	if err != nil {
		return false, err
	}
	return true, nil
}
