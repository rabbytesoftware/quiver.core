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
	"github.com/rabbytesoftware/quiver.core/internal/core/shutdown"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
)

type Container struct {
	Arrow      usecases.ArrowUsecase
	Runtime    usecases.RuntimeUsecase
	Collection usecases.CollectionUsecase
	Search     usecases.SearchUsecase
	// Discovery is nil when the container was built without a vault or a
	// manifold: there is nothing for a discovery pass to parse with or write
	// to, so the routes report it rather than half-running.
	Discovery usecases.DiscoveryUsecase
	Hub       *hub.Hub

	repos    *repositories.Container
	arrowsDB *gormdb.DB
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

// discardDB releases a read-model handle opened by a New that failed afterwards,
// so a half-built container never leaves a SQLite file open with no owner.
func discardDB(db *gormdb.DB) {
	if err := adapterSqlite.CloseDB(db); err != nil {
		slog.Warn("app container: close arrows db after failed construction", "err", err)
	}
}

// discardRepos releases everything a successfully built repositories.Container
// owns — including the collections database it opened — plus the arrows handle.
func discardRepos(repos *repositories.Container, db *gormdb.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdown.DiscardTimeout)
	defer cancel()

	if err := repos.Shutdown(ctx); err != nil {
		slog.Warn("app container: release repositories after failed construction", "err", err)
	}
	discardDB(db)
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
		engines.Providers,
	)
	if err != nil {
		discardDB(db)
		return nil, fmt.Errorf("app container: repositories: %w", err)
	}

	if err := repos.RegisterHubProjections(h); err != nil {
		discardRepos(repos, db)
		return nil, fmt.Errorf("app container: hub projections: %w", err)
	}

	uc, err := usecases.New(repos, engines.Manifold, engines.Vault)
	if err != nil {
		discardRepos(repos, db)
		return nil, fmt.Errorf("app container: usecases: %w", err)
	}

	return &Container{
		Arrow:      uc.Arrow,
		Runtime:    uc.Runtime,
		Collection: uc.Collection,
		Search:     uc.Search,
		Discovery:  uc.Discovery,
		Hub:        h,
		repos:      repos,
		arrowsDB:   db,
	}, nil
}

// newAsynx builds an asynx instance wired to s's paired event and snapshot
// stores.
//
// WorkersPerShard is left at the asynx default. Pinning it to 1 deadlocks arrow
// deletion: newAsynx is called once per aggregate, so arrow and runtime each get
// their own processor and shard pool, and the deadlock is a circular wait across
// those two instances, not within one shard. An arrow worker running the
// synchronous OnArrowRemoved cascade (repositories/container.go) blocks inside
// PublishSync on a send into axRuntime, while a runtime worker is already
// blocked on a send of its own. With one worker per shard there is nobody left
// to make progress.
//
// The cost of reverting is real, not zero. Per asynx's ShardingOpts docs,
// WorkersPerShard 1 serializes load-validate-write per shard so same-aggregate
// commands cannot conflict; above 1 the loser of a race returns
// ErrPipelineFailed. Every call site in arrow.go and runtime.go maps that to
// apperrors.ErrStateViolation, and BeginStop is the only one that retries.
//
// This revert widens the deadlock window rather than closing it: the circular
// wait still exists, it now just needs all 8 workers blocked instead of 1. The
// actual fix is to make the forget-cascade non-blocking.
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
