// Package store provides generic, swappable storage implementations.
// Any Quiver component can depend on these interfaces and swap implementations freely.
package store

// Store is a generic read-model keyed by integer ID.
type Store[T any] interface {
	Save(item T) error
	Delete(id int) error
	FindByID(id int) (*T, error)
	FindAll() ([]T, error)
}
