// Package store provides generic, swappable storage implementations.
// Any Quiver component can depend on these interfaces and swap implementations freely.
package store

import "context"

// Store is a generic read-model keyed by a comparable key K.
type Store[T any, K comparable] interface {
	Save(
		ctx context.Context,
		item T,
	) error
	Delete(
		ctx context.Context,
		id K,
	) error
	FindByKey(
		ctx context.Context,
		id K,
	) (*T, error)
	FindAll(ctx context.Context) ([]T, error)
	// Close releases the backing handle. Implementations that were handed an
	// already-open handle leave it alone — only the opener closes it.
	Close() error
}
