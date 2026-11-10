package bind

import (
	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"gorm.io/gorm"
)

// Bound is an asynx instance that also accepts durable and sync routes after
// it was built. Obtain one from Build.
type Bound[T any] interface {
	asynx.Asynx[T]

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

	// Host returns the events database where a read model can live and
	// commit with its events, or nil when the event store cannot host one.
	Host() *gorm.DB
}
