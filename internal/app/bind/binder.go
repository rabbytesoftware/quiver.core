package bind

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
)

// Binder routes the events of one asynx instance to handlers registered after
// the instance was built.
type Binder[T any] interface {
	// Durable adds a durable route. The pattern is what asynx's Subscribe
	// takes: an exact event name, or a regular expression (wrap a glob with
	// asynx.Topic). It fails when the pattern is not a valid expression.
	Durable(
		pattern string,
		fn asynxModels.DurableHandler[T],
	) error

	// Sync adds a sync-projection route. The pattern is what asynx's Subscribe
	// takes. It fails when the pattern is not a valid expression.
	Sync(
		pattern string,
		fn SyncFn[T],
	) error

	// Ready releases the durable deliveries that were waiting for wiring. It
	// is safe to call more than once.
	Ready()

	// DispatchDurable is the catch-all durable subscriber. It waits for Ready,
	// then runs every matching route; the errors are returned together after
	// all ran, so one failing route is retried without starving its siblings
	// of the attempt.
	DispatchDurable(
		ctx context.Context,
		evt asynxModels.Event[T],
	) error

	// DispatchSync is the catch-all sync projection. Routes run in
	// registration order inside the event's transaction and the first error
	// aborts it. It does not wait for Ready: no command reaches an aggregate
	// before its repositories are wired, and blocking here would hold the
	// transaction open.
	DispatchSync(
		ctx context.Context,
		tx asynxModels.Tx,
		evt asynxModels.Event[T],
	) error
}
