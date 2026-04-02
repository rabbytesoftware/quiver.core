package store

import "sync"

type memoryStore[T any] struct {
	mu    sync.RWMutex
	data  map[int]T
	keyFn func(T) int
}

// NewMemory returns a generic in-memory Store.
// keyFn extracts the integer primary key from an item.
func NewMemory[T any](keyFn func(T) int) Store[T] {
	return &memoryStore[T]{
		data:  make(map[int]T),
		keyFn: keyFn,
	}
}

func (m *memoryStore[T]) Save(item T) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.keyFn(item)
	m.data[id] = item
	return nil
}

func (m *memoryStore[T]) Delete(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, id)
	return nil
}

func (m *memoryStore[T]) FindByID(id int) (*T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.data[id]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (m *memoryStore[T]) FindAll() ([]T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]T, 0, len(m.data))
	for _, item := range m.data {
		result = append(result, item)
	}
	return result, nil
}
