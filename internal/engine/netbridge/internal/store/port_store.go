package store

import (
	"context"

	adapterstore "github.com/rabbytesoftware/quiver/internal/adapter/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

type portStore struct {
	inner adapterstore.Store[ports.PortAllocation, int]
}

func (p *portStore) Save(ctx context.Context, item ports.PortAllocation) error {
	return p.inner.Save(ctx, item)
}

func (p *portStore) Delete(ctx context.Context, id int) error {
	return p.inner.Delete(ctx, id)
}

func (p *portStore) FindByKey(ctx context.Context, id int) (*ports.PortAllocation, error) {
	return p.inner.FindByKey(ctx, id)
}

func (p *portStore) FindAll(ctx context.Context) ([]ports.PortAllocation, error) {
	return p.inner.FindAll(ctx)
}

func (p *portStore) FindByPort(ctx context.Context, port int) (*ports.PortAllocation, error) {
	return p.inner.FindByKey(ctx, port)
}

func (p *portStore) FindByOwner(ctx context.Context, ownerKey string) ([]ports.PortAllocation, error) {
	all, err := p.inner.FindAll(ctx)
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
