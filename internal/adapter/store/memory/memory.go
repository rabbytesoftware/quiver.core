package store

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/adapter/store"
)

type memoryStore[T any, K comparable] struct {
	mu    sync.RWMutex
	data  map[K]T
	keyFn func(T) K
}

// NewMemory returns a generic in-memory Store.
// keyFn extracts the primary key from an item.
func NewMemory[T any, K comparable](keyFn func(T) K) store.Store[T, K] {
	return &memoryStore[T, K]{
		data:  make(map[K]T),
		keyFn: keyFn,
	}
}

func (m *memoryStore[T, K]) Save(_ context.Context, item T) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[m.keyFn(item)] = item
	return nil
}

func (m *memoryStore[T, K]) Delete(_ context.Context, id K) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, id)
	return nil
}

func (m *memoryStore[T, K]) FindByKey(_ context.Context, id K) (*T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.data[id]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (m *memoryStore[T, K]) FindAll(_ context.Context) ([]T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]T, 0, len(m.data))
	for _, item := range m.data {
		result = append(result, item)
	}
	return result, nil
}
