// Package store provides generic, swappable storage implementations.
// Any Quiver component can depend on these interfaces and swap implementations freely.
package store

// Store is a generic read-model keyed by a comparable key K.
type Store[T any, K comparable] interface {
	Save(item T) error
	Delete(id K) error
	FindByKey(id K) (*T, error)
	FindAll() ([]T, error)
}
