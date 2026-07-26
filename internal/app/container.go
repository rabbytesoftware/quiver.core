package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

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
}

func (c *Container) Start(ctx context.Context) {
	c.Runtime.Start(ctx)
}

// Shutdown drains every aggregate the app layer owns. It must run before the
// event and snapshot stores are closed, otherwise in-flight writes land on a
// closed database.
func (c *Container) Shutdown(ctx context.Context) error {
	if c.repos == nil {
		return nil
	}
	return c.repos.Shutdown(ctx)
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
	}, nil
}

// newAsynx builds an asynx instance wired to s's paired event and snapshot
// stores.
//
// WorkersPerShard is pinned to 1 as a correctness knob, not a throughput
// dial: the v0.8 write path is optimistic concurrency, so with the default
// of 8 workers two commands racing on the same aggregate can collide and the
// loser surfaces ErrPipelineFailed as a phantom HTTP 409. Serializing to one
// worker per shard removes that race entirely. Do not raise this back to 8
// without also handling ErrPipelineFailed with a read-and-retry at the call
// site.
func newAsynx[T any](
	s adapter.Stores,
) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(s.Events).
		WithSnapshotStore(s.Snapshots).
		WithShardingOpts(asynx.ShardingOpts{
			Shards:          8,
			QueueDepth:      1000,
			WorkersPerShard: 1,
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
