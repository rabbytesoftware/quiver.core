package arrow

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowproj "github.com/rabbytesoftware/quiver/internal/app/arrow/projections"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

type Builder struct {
	eventStore        asynxModels.Store
	runtimeEventStore asynxModels.Store
	catalog           arrowstore.ArrowCatalog
	vault             vault.Vault
	manifold          manifold.Manifold
	deptree           deptree.DepTree
	netbridge         netbridge.Netbridge
	wizard            wizard.Wizard
	os                string
}

func NewArrowBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) WithEventStore(es asynxModels.Store) *Builder {
	b.eventStore = es
	return b
}

func (b *Builder) WithRuntimeEventStore(es asynxModels.Store) *Builder {
	b.runtimeEventStore = es
	return b
}

func (b *Builder) WithCatalog(c arrowstore.ArrowCatalog) *Builder {
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

func (b *Builder) WithDepTree(dt deptree.DepTree) *Builder {
	b.deptree = dt
	return b
}

func (b *Builder) WithNetbridge(nb netbridge.Netbridge) *Builder {
	b.netbridge = nb
	return b
}

func (b *Builder) WithWizard(w wizard.Wizard) *Builder {
	b.wizard = w
	return b
}

func (b *Builder) WithOS(os string) *Builder {
	b.os = os
	return b
}

// Build constructs and returns an ArrowService
func (b *Builder) Build(ctx context.Context) (ArrowService, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("arrow builder: event store is required")
	}

	axArrow, err := newAsynxArrow(b.eventStore)
	if err != nil {
		return nil, err
	}

	runtimeES := b.runtimeEventStore
	if runtimeES == nil {
		runtimeES = b.eventStore
	}

	axRuntime, err := newAsynxRuntime(runtimeES)
	if err != nil {
		return nil, err
	}

	catalog := b.catalog
	if catalog == nil {
		var err error
		catalog, err = arrowstore.NewArrowCatalog()
		if err != nil {
			return nil, err
		}
	}

	svc := &arrowService{
		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
		catalog:      catalog,
		vault:        b.vault,
		manifold:     b.manifold,
		deptree:      b.deptree,
		netbridge:    b.netbridge,
		wizard:       b.wizard,
		os:           b.os,
	}

	_, err = svc.asynxArrow.Subscribe("arrow.added", arrowproj.OnArrowAdded(catalog))
	if err != nil {
		return nil, err
	}
	_, err = svc.asynxArrow.Subscribe("arrow.updated", arrowproj.OnArrowUpdated(catalog))
	if err != nil {
		return nil, err
	}
	_, err = svc.asynxArrow.Subscribe("arrow.removed", arrowproj.OnArrowRemoved(catalog))
	if err != nil {
		return nil, err
	}

	_, err = svc.asynxRuntime.Subscribe("runtime.mark_stopping", arrowproj.StopCoordinator(b.wizard))
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// newAsynxArrow creates and returns a new Asynx instance for Arrow aggregates
func newAsynxArrow(es asynxModels.Store) (asynx.Asynx[domain.Arrow], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx arrow: event store is required")
	}

	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}

// newAsynxRuntime creates and returns a new Asynx instance for ArrowRuntime aggregates
func newAsynxRuntime(es asynxModels.Store) (asynx.Asynx[domainRuntime.ArrowRuntime], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx runtime: event store is required")
	}

	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}
