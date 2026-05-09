package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	adapterSqlite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/app/repositories"
	"github.com/rabbytesoftware/quiver/internal/app/usecases"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Container struct {
	Arrow      usecases.ArrowUsecase
	Runtime    usecases.RuntimeUsecase
	Collection usecases.CollectionUsecase
	Hub        *hub.Hub
}

func (c *Container) Start(ctx context.Context) {
	c.Runtime.Start(ctx)
}

func (c *Container) Shutdown(ctx context.Context) error {
	return c.Runtime.Shutdown(ctx)
}

type appOpts struct{ homeDir string }

type Option func(*appOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(dir string) Option {
	return func(o *appOpts) { o.homeDir = dir }
}

// New constructs Arrow, Runtime, and Quiver usecases wired to the provided engine
// and adapter containers. Callers are responsible for opening and managing the event stores.
func New(
	engines *engine.Container,
	adapters *adapter.Container,
	opts ...Option,
) (*Container, error) {
	cfg := appOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	os := domain.CurrentOS()

	var storePath string
	var err error
	if cfg.homeDir != "" {
		storePath, err = paths.StoreAt(cfg.homeDir)
	} else {
		storePath, err = paths.Store()
	}
	if err != nil {
		return nil, fmt.Errorf("app container: store path: %w", err)
	}

	axArrow, err := newAsynx[domain.Arrow](adapters.ArrowES)
	if err != nil {
		return nil, fmt.Errorf("app container: asynx arrow: %w", err)
	}

	axRuntime, err := newAsynx[domainRuntime.ArrowRuntime](adapters.RuntimeES)
	if err != nil {
		return nil, fmt.Errorf("app container: asynx runtime: %w", err)
	}

	axCollection, err := newAsynx[domain.Collection](adapters.QuiverES)
	if err != nil {
		return nil, fmt.Errorf("app container: asynx quiver: %w", err)
	}

	db, err := adapterSqlite.OpenDB(filepath.Join(storePath, "arrows.db"))
	if err != nil {
		return nil, fmt.Errorf("app container: open db: %w", err)
	}

	quiverDBPath := filepath.Join(storePath, "collections.db")

	h := hub.NewHub()

	repos, err := repositories.New(
		db,
		axArrow,
		axRuntime,
		axCollection,
		quiverDBPath,
		engines.Vault,
		engines.Manifold,
		engines.Wizard,
		os,
		h,
	)
	if err != nil {
		return nil, fmt.Errorf("app container: repositories: %w", err)
	}

	if err := repos.RegisterHubProjections(h); err != nil {
		return nil, fmt.Errorf("app container: hub projections: %w", err)
	}

	uc, err := usecases.New(repos, engines.Manifold, engines.Vault)
	if err != nil {
		return nil, fmt.Errorf("app container: usecases: %w", err)
	}

	return &Container{
		Arrow:      uc.Arrow,
		Runtime:    uc.Runtime,
		Collection: uc.Collection,
		Hub:        h,
	}, nil
}

func newAsynx[T any](es asynxModels.Store) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}
