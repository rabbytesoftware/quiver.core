package store

import (
	"sync"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

type memoryStore struct {
	mu   sync.RWMutex
	data map[int]ports.PortAllocation
}

// NewMemory returns an in-memory Store implementation.
// Suitable for tests and as a fallback when no database path is configured.
func NewMemory() Store {
	return &memoryStore{
		data: make(map[int]ports.PortAllocation),
	}
}

func (r *memoryStore) Save(
	allocation ports.PortAllocation,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[allocation.Port] = allocation
	return nil
}

func (r *memoryStore) Delete(
	port int,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.data, port)
	return nil
}

func (r *memoryStore) FindByPort(
	port int,
) (*ports.PortAllocation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	alloc, ok := r.data[port]
	if !ok {
		return nil, nil
	}
	return &alloc, nil
}

func (r *memoryStore) FindByOwner(
	ownerKey string,
) ([]ports.PortAllocation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ports.PortAllocation, 0, 4)
	for _, alloc := range r.data {
		if alloc.OwnerKey == ownerKey {
			result = append(result, alloc)
		}
	}
	return result, nil
}
