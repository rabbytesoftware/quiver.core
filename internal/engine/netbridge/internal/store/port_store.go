package store

import (
	adapterstore "github.com/rabbytesoftware/quiver/internal/adapter/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

type portStore struct {
	inner adapterstore.Store[ports.PortAllocation, int]
}

func (p *portStore) Save(item ports.PortAllocation) error {
	return p.inner.Save(item)
}

func (p *portStore) Delete(id int) error {
	return p.inner.Delete(id)
}

func (p *portStore) FindByKey(id int) (*ports.PortAllocation, error) {
	return p.inner.FindByKey(id)
}

func (p *portStore) FindAll() ([]ports.PortAllocation, error) {
	return p.inner.FindAll()
}

func (p *portStore) FindByPort(port int) (*ports.PortAllocation, error) {
	return p.inner.FindByKey(port)
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
