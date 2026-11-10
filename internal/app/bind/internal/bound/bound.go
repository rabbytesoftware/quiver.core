package bound

import (
	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/app/bind/internal/binder"
)

// Bound is an asynx instance paired with the Binder its builder forwarded to
// and, when the event store can host read models, the database they live in.
type Bound[T any] struct {
	asynx.Asynx[T]
	binder *binder.Binder[T]
	db     *gorm.DB
}

// New pairs ax with its binder and hosting database (nil when the store cannot
// host read models).
func New[T any](
	ax asynx.Asynx[T],
	b *binder.Binder[T],
	db *gorm.DB,
) *Bound[T] {
	return &Bound[T]{Asynx: ax, binder: b, db: db}
}

// Durable adds a durable route to the binder.
func (b *Bound[T]) Durable(
	pattern string,
	fn asynxModels.DurableHandler[T],
) error {
	return b.binder.Durable(pattern, fn)
}

// Sync adds a sync-projection route to the binder.
func (b *Bound[T]) Sync(
	pattern string,
	fn binder.SyncFn[T],
) error {
	return b.binder.Sync(pattern, fn)
}

// Ready releases the instance's waiting durable deliveries.
func (b *Bound[T]) Ready() {
	b.binder.Ready()
}

// Host returns the events database, or nil when the store cannot host one.
func (b *Bound[T]) Host() *gorm.DB {
	return b.db
}
