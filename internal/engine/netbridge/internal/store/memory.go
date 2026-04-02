package store

import (
	"github.com/rabbytesoftware/quiver/internal/adapter/store"
	adaptermem "github.com/rabbytesoftware/quiver/internal/adapter/store/memory"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

// NewPortMemory returns an in-memory PortStore.
func NewPortMemory() PortStore {
	return &portStore{
		inner: adaptermem.NewMemory(
			func(pa ports.PortAllocation) int { return pa.Port },
		),
	}
}

type portStore struct {
	inner store.Store[ports.PortAllocation]
}

func (p *portStore) Save(item ports.PortAllocation) error {
	return p.inner.Save(item)
}

func (p *portStore) Delete(id int) error {
	return p.inner.Delete(id)
}

func (p *portStore) FindByID(id int) (*ports.PortAllocation, error) {
	return p.inner.FindByID(id)
}

func (p *portStore) FindAll() ([]ports.PortAllocation, error) {
	return p.inner.FindAll()
}

func (p *portStore) FindByPort(port int) (*ports.PortAllocation, error) {
	return p.inner.FindByID(port)
}

func (p *portStore) FindByOwner(ownerKey string) ([]ports.PortAllocation, error) {
	all, err := p.inner.FindAll()
	if err != nil {
		return nil, err
	}

	result := make([]ports.PortAllocation, 0)
	for _, alloc := range all {
		if alloc.OwnerKey == ownerKey {
			result = append(result, alloc)
		}
	}
	return result, nil
}
