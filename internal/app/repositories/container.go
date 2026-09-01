package repositories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	repoarrow "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/cascade"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection"
	repoconfig "github.com/rabbytesoftware/quiver.core/internal/app/repositories/config"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/shutdown"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

type Container struct {
	Arrow      repoarrow.Arrow
	Runtime    runtime.Runtime
	Collection collection.Collection
	Graph      graph.Graph
	Cascade    cascade.Cascade
	Discovery  discovery.Discovery
	Config     repoconfig.Config
}

func New(
	db *gormdb.DB,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	axCollection asynx.Asynx[domain.Collection],
	collectionDBPath string,
	v vault.Vault,
	m manifold.Manifold,
	w wizardPkg.Wizard,
	os domain.OS,
	hub apphub.WebSocketHub,
	providers []provider.Provider,
) (*Container, error) {
	g, err := graph.New(db, os, m, resolveManifestFrom(axArrow, m))
	if err != nil {
		return nil, fmt.Errorf("repositories: graph: %w", err)
	}

	cat, err := repoarrow.New(db, axArrow, v, m, hub)
	if err != nil {
		return nil, fmt.Errorf("repositories: arrow: %w", err)
	}

	coll, err := collection.NewFromDBPath(axCollection, collectionDBPath, v, m)
	if err != nil {
		return nil, fmt.Errorf("repositories: quiver: %w", err)
	}

	rt, err := runtime.New(
		arrowGetter(axArrow),
		axRuntime,
		w,
		v,
		cat.MarkInstalled,
		cat.MarkUninstalled,
		cat.MarkLastUsed,
		dependentsChecker(g),
		catalogLister(cat),
		os,
	)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: runtime: %w", err)
	}

	fc, err := cascade.New(db, rt.Forget)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: cascade: %w", err)
	}

	disc, err := newDiscovery(providers, m, v, cat)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: discovery: %w", err)
	}

	c := &Container{
		Arrow:      cat,
		Runtime:    rt,
		Collection: coll,
		Graph:      g,
		Cascade:    fc,
		Discovery:  disc,
		Config:     repoconfig.New(),
	}

	if err := c.wireCallbacks(); err != nil {
		discardCollection(coll)
		return nil, err
	}

	return c, nil
}

// arrowGetter hands the runtime a read of the arrow aggregate without handing
// it the aggregate.
func arrowGetter(
	axArrow asynx.Asynx[domain.Arrow],
) func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
	return func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		got, err := axArrow.Get(ctx, ns.String())
		if err != nil {
			return nil, err
		}
		return &got, nil
	}
}

// dependentsChecker asks the graph whether anything still needs ns. No arrow is
// excluded: the runtime asks on behalf of nobody in particular.
func dependentsChecker(
	g graph.Graph,
) runtime.HasDependentsFn {
	return func(ctx context.Context, ns domain.Namespace) (bool, error) {
		return g.HasDependents(ctx, ns, domain.Namespace(""))
	}
}

func catalogLister(
	cat repoarrow.Arrow,
) func(ctx context.Context) ([]models.ArrowView, error) {
	return func(ctx context.Context) ([]models.ArrowView, error) {
		return cat.List(ctx, nil)
	}
}

// newDiscovery reads the search settings once, per CLAUDE.md §15.2, and gives
// the pipeline a catalog lookup so an arrow the machine already knows is
// flagged rather than dropped. Discovery cannot verify anything without a
// manifold to parse with and a vault to write to, so a container built without
// them has no discovery rather than a half-built one.
func newDiscovery(
	providers []provider.Provider,
	m manifold.Manifold,
	v vault.Vault,
	cat repoarrow.Arrow,
) (discovery.Discovery, error) {
	if m == nil || v == nil {
		return nil, nil
	}

	search := config.GetSearch()

	return discovery.New(providers, m, v, catalogHas(cat), discovery.Config{
		Topics:           metadata.GetDiscovery().Topics,
		PerProviderLimit: search.PerProviderLimit,
		FetchConcurrency: search.FetchConcurrency,
	})
}

func catalogHas(
	cat repoarrow.Arrow,
) discovery.KnownFn {
	return func(ctx context.Context, ns domain.Namespace) (bool, error) {
		_, err := cat.Get(ctx, ns)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("catalog has %s: %w", ns, err)
	}
}

// discardCollection closes the collections database opened by NewFromDBPath when
// a later step of New fails, so a half-built container never leaves the file open
// with no owner.
func discardCollection(coll collection.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdown.DiscardTimeout)
	defer cancel()

	if err := coll.Shutdown(ctx); err != nil {
		slog.Warn("repositories: close collection store after failed construction", "err", err)
	}
}

// Shutdown drains every aggregate, blocking until in-flight commands have been
// persisted or ctx expires.
//
// Cascade drains first: its background goroutine calls Runtime.Forget, so it
// must finish (or be cut off) before Runtime itself shuts down underneath it.
//
// Runtime drains next and Arrow last. When an install finishes, the runtime
// reaction's onEnd writes arrow.MarkInstalled and only then commits
// EndExecution (runtime/internal/hooks.go). Draining Arrow first would lose
// MarkInstalled while EndExecution still commits, leaving a ready runtime whose
// arrow carries no installed ref — nothing reconciles that. Draining Runtime
// first makes EndExecution the write that fails instead, so the runtime stays
// in `installing`, which RecoverTransients re-drives on the next boot
// (runtime/internal/recovery.go).
//
// Every phase runs even when an earlier one fails, and each gets its own share of
// ctx rather than all three sharing it: an arrow whose process refuses to die
// makes the runtime drain spend the whole budget, and with a shared context
// Collection and Arrow would then be handed a dead one and skip their drain
// entirely — right before the adapters close the databases under them.
func (c *Container) Shutdown(ctx context.Context) error {
	return shutdown.Split(ctx, "repositories", []shutdown.Phase{
		{Name: "cascade shutdown", Run: c.Cascade.Shutdown},
		{Name: "runtime shutdown", Run: c.Runtime.Shutdown},
		{Name: "collection shutdown", Run: c.Collection.Shutdown},
		{Name: "arrow shutdown", Run: c.Arrow.Shutdown},
	})
}

// RecoverForgetCascade finishes any forget cascade a prior crash left pending.
// Call once at boot, before anything else touches the namespaces involved —
// mirrors runtimeRepository.Start's use of RecoverTransients
// (runtime/internal/recovery.go) for the same crash-recovery role on the other
// side of an arrow removal.
func (c *Container) RecoverForgetCascade(ctx context.Context) {
	if err := c.Cascade.Drain(ctx); err != nil {
		slog.ErrorContext(ctx, "repositories: forget cascade recovery", "err", err)
	}
}

// wireCallbacks runs before any other registration, so the dependency graph is
// the first reaction to every arrow event. The arrow repository invokes
// callbacks in registration order and only makes the arrow readable afterwards,
// which is what makes "readable in the catalog" imply "its edges exist".
func (c *Container) wireCallbacks() error {
	if err := c.Arrow.OnArrowAdded(func(ctx context.Context, ns domain.Namespace, a domain.Arrow) error {
		return c.Graph.SyncDependencies(ctx, ns, &a)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowAdded: %w", err)
	}

	if err := c.Arrow.OnArrowUpdated(func(ctx context.Context, ns domain.Namespace, a *domain.Arrow) error {
		return c.Graph.SyncDependencies(ctx, ns, a)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowUpdated: %w", err)
	}

	// An upgrade replaces the manifest, so it replaces the edges too. graph no
	// longer projects this itself, and the usecase reaction that runs after
	// this one reads the edges back.
	if err := c.Arrow.OnArrowUpgraded(func(ctx context.Context, a domain.Arrow) error {
		return c.Graph.SyncDependencies(ctx, a.Namespace, &a)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowUpgraded: %w", err)
	}

	if err := c.Arrow.OnArrowRemoved(func(ctx context.Context, ns domain.Namespace) error {
		if err := c.Graph.RemoveDependencies(ctx, ns); err != nil {
			return err
		}
		return c.Cascade.Enqueue(ctx, ns)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowRemoved: %w", err)
	}

	return nil
}

func (c *Container) RegisterHubProjections(hub apphub.WebSocketHub) error {
	if err := c.Runtime.OnRuntimeBegun(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeBegun: %w", err)
	}

	if err := c.Runtime.OnRuntimeEnded(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeEnded: %w", err)
	}

	if err := c.Runtime.OnRuntimeRecovered(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeRecovered: %w", err)
	}

	if err := c.Runtime.OnRuntimeDetached(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeDetached: %w", err)
	}

	if err := c.Runtime.OnRuntimePIDRecorded(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimePIDRecorded: %w", err)
	}

	if err := c.Runtime.OnRuntimeOutdated(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeOutdated: %w", err)
	}

	if err := c.Runtime.OnRuntimeOutdatedCleared(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeOutdatedCleared: %w", err)
	}

	if err := c.Runtime.OnRuntimeStepAdvanced(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeStepAdvanced: %w", err)
	}

	if err := c.Collection.OnCollectionFollowed(func(_ context.Context, q domain.Collection) {
		hub.BroadcastCollection(apphub.CollectionEvent{Kind: apphub.CatalogUpserted, Collection: q})
	}); err != nil {
		return fmt.Errorf("repositories: hub OnCollectionFollowed: %w", err)
	}

	if err := c.Collection.OnCollectionUnfollowed(func(_ context.Context, ns domain.Namespace) {
		hub.BroadcastCollection(apphub.CollectionEvent{Kind: apphub.CatalogRemoved, Collection: domain.Collection{Namespace: ns}})
	}); err != nil {
		return fmt.Errorf("repositories: hub OnCollectionUnfollowed: %w", err)
	}

	return nil
}

// resolveManifestFrom builds a resolveManifest func for graph.New, falling back to manifold.
func resolveManifestFrom(
	axArrow asynx.Asynx[domain.Arrow],
	m manifold.Manifold,
) func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
	return func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		got, err := axArrow.Get(ctx, ns.String())
		if err == nil {
			return &got, nil
		}
		if !isNotFound(err) {
			return nil, fmt.Errorf("resolve manifest: asynx: %w", err)
		}
		if m == nil {
			return nil, fmt.Errorf("resolve manifest: not found: %s", ns)
		}
		arrow, _, _, fetchErr := m.ResolveArrow(ctx, ns)
		if fetchErr != nil {
			return nil, fmt.Errorf("resolve manifest: %w", fetchErr)
		}
		return arrow, nil
	}
}

func isNotFound(err error) bool {
	return err != nil && err.Error() == asynxModels.ErrNotFound.Error()
}
