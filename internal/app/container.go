package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	adapterSqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
)

type Container struct {
	Arrow      usecases.ArrowUsecase
	Runtime    usecases.RuntimeUsecase
	Collection usecases.CollectionUsecase
	Hub        *hub.Hub
	repos      *repositories.Container
	arrowsDB   *gormdb.DB
}

func (c *Container) Start(ctx context.Context) {
	c.Runtime.Start(ctx)
}

// Shutdown drains every aggregate the app layer owns, then closes the arrows
// read model this container opened. It must run before the event and snapshot
// stores are closed, otherwise in-flight writes land on a closed database.
//
// arrows.db backs both the arrow and the graph read models, so it closes here
// rather than in either repository — and only once repos.Shutdown has drained
// the arrow aggregate, since both read models are fed by its projections.
func (c *Container) Shutdown(ctx context.Context) error {
	var errs []error

	if err := c.shutdownRepos(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := c.closeArrowsDB(); err != nil {
		errs = append(errs, fmt.Errorf("app container: close arrows db: %w", err))
	}

	return errors.Join(errs...)
}

func (c *Container) shutdownRepos(ctx context.Context) error {
	if c.repos == nil {
		return nil
	}
	return c.repos.Shutdown(ctx)
}

func (c *Container) closeArrowsDB() error {
	if c.arrowsDB == nil {
		return nil
	}
	return adapterSqlite.CloseDB(c.arrowsDB)
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

	axArrow, err := newAsynx[domain.Arrow](adapters.Arrow)
	if err != nil {
		return nil, fmt.Errorf("app container: asynx arrow: %w", err)
	}

	axRuntime, err := newAsynx[domainRuntime.ArrowRuntime](adapters.Runtime)
	if err != nil {
		return nil, fmt.Errorf("app container: asynx runtime: %w", err)
	}

	axCollection, err := newAsynx[domain.Collection](adapters.Quiver)
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
		repos:      repos,
		arrowsDB:   db,
	}, nil
}

// newAsynx builds an asynx instance wired to s's paired event and snapshot
// stores.
//
// WorkersPerShard is left at the asynx default because shards must keep more
// than one worker: the OnArrowRemoved hook in repositories/container.go is a
// synchronous blocking cascade into a second aggregate on the same shard, so a
// single worker per shard has no slack to absorb it and arrow deletion
// deadlocks.
func newAsynx[T any](
	s adapter.Stores,
) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(s.Events).
		WithSnapshotStore(s.Snapshots).
		WithShardingOpts(asynx.ShardingOpts{
			Shards:     8,
			QueueDepth: 1000,
		}).
		WithCorruptionHook(func(err error) {
			slog.Error("asynx snapshot unreadable; falling back to cold replay", "err", err)
		}).
		WithPanicHandler(func(ctx context.Context, evt asynxModels.Event[T], p any) {
			slog.ErrorContext(ctx, "asynx projection panic; read model may be stale",
				"aggregate", evt.AggregateID, "event", evt.EventName, "version", evt.Version, "panic", p)
		}).
		WithPublishErrorHandler(func(ctx context.Context, evt asynxModels.Event[T], err error) {
			slog.ErrorContext(ctx, "asynx publish failed; event is durable but was not delivered",
				"aggregate", evt.AggregateID, "event", evt.EventName, "version", evt.Version, "err", err)
		}).
		Build()
}
