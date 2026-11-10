package bind

import (
	"github.com/char2cs/asynx"
	"gorm.io/gorm"
)

// Bound is an asynx instance that also accepts durable and sync routes after
// it was built.
type Bound[T any] interface {
	asynx.Asynx[T]

	// Binder returns the Binder the instance's catch-all registrations
	// forward to.
	Binder() Binder[T]

	// Ready releases the instance's waiting durable deliveries.
	Ready()

	// Host returns the events database where a read model can live and
	// commit with its events, or nil when the event store cannot host one.
	Host() *gorm.DB
}
