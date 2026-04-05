package quiver

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	quiverproj "github.com/rabbytesoftware/quiver/internal/app/quiver/projections"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

type Builder struct {
	eventStore asynxModels.Store
	catalog    quiverstore.QuiverCatalog
	vault      vault.Vault
	manifold   manifold.Manifold
	os         string
}

func NewQuiverBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) WithEventStore(es asynxModels.Store) *Builder {
	b.eventStore = es
	return b
}

func (b *Builder) WithCatalog(c quiverstore.QuiverCatalog) *Builder {
	b.catalog = c
	return b
}

func (b *Builder) WithVault(v vault.Vault) *Builder {
	b.vault = v
	return b
}

func (b *Builder) WithManifold(m manifold.Manifold) *Builder {
	b.manifold = m
	return b
}

func (b *Builder) WithOS(os string) *Builder {
	b.os = os
	return b
}

// Build constructs and returns a QuiverService
func (b *Builder) Build(ctx context.Context) (QuiverService, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("quiver builder: event store is required")
	}

	axQuiver, err := newAsynxQuiver(b.eventStore)
	if err != nil {
		return nil, err
	}

	catalog := b.catalog
	if catalog == nil {
		var err error
		catalog, err = quiverstore.NewQuiverCatalog()
		if err != nil {
			return nil, err
		}
	}

	svc := &quiverService{
		asynxQuiver: axQuiver,
		catalog:     catalog,
		vault:       b.vault,
		manifold:    b.manifold,
		os:          b.os,
	}

	_, err = svc.asynxQuiver.Subscribe("quiver.added", quiverproj.OnQuiverAdded(catalog))
	if err != nil {
		return nil, err
	}
	_, err = svc.asynxQuiver.Subscribe("quiver.updated", quiverproj.OnQuiverUpdated(catalog))
	if err != nil {
		return nil, err
	}
	_, err = svc.asynxQuiver.Subscribe("quiver.removed", quiverproj.OnQuiverRemoved(catalog))
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// newAsynxQuiver creates and returns a new Asynx instance for Quiver aggregates
func newAsynxQuiver(es asynxModels.Store) (asynx.Asynx[domain.Quiver], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx quiver: event store is required")
	}

	ax, err := asynx.New[domain.Quiver]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}
