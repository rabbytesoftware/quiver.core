package bind

import (
	"github.com/char2cs/asynx"
	"gorm.io/gorm"
)

type bound[T any] struct {
	asynx.Asynx[T]
	binder Binder[T]
	db     *gorm.DB
}

// Wrap pairs ax with the Binder its Builder forwarded to and, when the event
// store can host read models, the database they live in (nil otherwise).
func Wrap[T any](
	ax asynx.Asynx[T],
	binder Binder[T],
	db *gorm.DB,
) Bound[T] {
	return &bound[T]{Asynx: ax, binder: binder, db: db}
}

func (b *bound[T]) Binder() Binder[T] {
	return b.binder
}

func (b *bound[T]) Ready() {
	b.binder.Ready()
}

func (b *bound[T]) Host() *gorm.DB {
	return b.db
}
