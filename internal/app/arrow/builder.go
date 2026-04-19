package arrow

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	appDeps "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Builder struct {
	engines           *engine.Container
	eventStore        asynxModels.Store
	runtimeEventStore asynxModels.Store
	catalog           catalog.Catalog
	os                domain.OS
	asynxArrow        asynx.Asynx[domain.Arrow]
	asynxRuntime      asynx.Asynx[domainRuntime.ArrowRuntime]
	hub               apphub.WebSocketHub
}

func NewArrowBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) WithEngines(e *engine.Container) *Builder {
	b.engines = e
	return b
}

func (b *Builder) WithEventStore(es asynxModels.Store) *Builder {
	b.eventStore = es
	return b
}

func (b *Builder) WithRuntimeEventStore(es asynxModels.Store) *Builder {
	b.runtimeEventStore = es
	return b
}

func (b *Builder) WithCatalog(c catalog.Catalog) *Builder {
	b.catalog = c
	return b
}

func (b *Builder) WithOS(os domain.OS) *Builder {
	b.os = os
	return b
}

func (b *Builder) WithAsynxArrow(axArrow asynx.Asynx[domain.Arrow]) *Builder {
	b.asynxArrow = axArrow
	return b
}

func (b *Builder) WithAsynxRuntime(axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]) *Builder {
	b.asynxRuntime = axRuntime
	return b
}

func (b *Builder) WithWebSocketHub(h apphub.WebSocketHub) *Builder {
	b.hub = h
	return b
}

// Build constructs and returns an ArrowService.
func (b *Builder) Build() (ArrowService, error) {
	if b.eventStore == nil && b.asynxArrow == nil {
		return nil, fmt.Errorf("arrow builder: event store is required")
	}

	axArrow := b.asynxArrow
	var err error
	if axArrow == nil {
		axArrow, err = newAsynxArrow(b.eventStore)
		if err != nil {
			return nil, err
		}
	}

	axRuntime := b.asynxRuntime
	if axRuntime == nil {
		runtimeES := b.runtimeEventStore
		if runtimeES == nil {
			runtimeES = b.eventStore
		}

		axRuntime, err = newAsynxRuntime(runtimeES)
		if err != nil {
			return nil, err
		}
	}

	e := b.engines
	if e == nil {
		e = &engine.Container{}
	}

	storePath, storePathErr := paths.Store()
	if storePathErr != nil {
		return nil, fmt.Errorf("arrow builder: %w", storePathErr)
	}
	db, dbErr := sqlite.OpenDB(filepath.Join(storePath, "arrows.db"))
	if dbErr != nil {
		return nil, fmt.Errorf("arrow builder: open db: %w", dbErr)
	}

	depEdgeStore, depEdgeErr := depsstore.NewDepEdgeStore(db)
	if depEdgeErr != nil {
		return nil, fmt.Errorf("arrow builder: dep edge store: %w", depEdgeErr)
	}

	resolveManifest := manifest.New(e.Vault, e.Manifold)

	// Use a pointer so the onUninstallSuccess callback can close over it nil-safely.
	var svc *arrowService

	exc, excErr := execution.New(
		axArrow,
		axRuntime,
		*e,
		b.os,
		func(ctx context.Context, ns domain.Namespace) {
			if svc != nil {
				svc.cleanupAfterUninstall(ctx, ns)
			}
		},
	)
	if excErr != nil {
		return nil, excErr
	}

	syncExc := exc.(execution.SyncExecutor)

	cat := b.catalog
	if cat == nil {
		arrowCat, storeErr := arrowstore.NewArrowCatalogFromDB(db)
		if storeErr != nil {
			return nil, storeErr
		}
		cat, err = catalog.New(axArrow, axRuntime, arrowCat, resolveManifest)
		if err != nil {
			return nil, err
		}
	}

	depSvc, depErr := appDeps.New(
		axArrow,
		e.DepTree,
		resolveManifest,
		depEdgeStore,
		func(ctx context.Context, ns domain.Namespace) error {
			return syncExc.ExecuteSync(ctx, ns, domain.MethodInstall, nil)
		},
		func(ctx context.Context, ns domain.Namespace) error {
			return exc.BeginExecution(ctx, ns, domain.MethodExecute, nil)
		},
		func(ctx context.Context, ns domain.Namespace) error {
			return syncExc.ExecuteSync(ctx, ns, domain.MethodUninstall, nil)
		},
	)
	if depErr != nil {
		return nil, depErr
	}

	if b.hub != nil {
		if err := registerWSProjections(axArrow, axRuntime, b.hub); err != nil {
			return nil, fmt.Errorf("arrow builder: %w", err)
		}
	}

	svc = &arrowService{
		catalog:         cat,
		deps:            depSvc,
		execution:       exc,
		resolveManifest: resolveManifest,
		asynxRuntime:    axRuntime,
		vault:           e.Vault,
		manifold:        e.Manifold,
	}

	return svc, nil
}

func newAsynxArrow(
	es asynxModels.Store,
) (asynx.Asynx[domain.Arrow], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx arrow: event store is required")
	}

	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}

func newAsynxRuntime(
	es asynxModels.Store,
) (asynx.Asynx[domainRuntime.ArrowRuntime], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx runtime: event store is required")
	}

	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}
