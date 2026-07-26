package repositories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	repoarrow "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

type Container struct {
	Arrow      repoarrow.Arrow
	Runtime    runtime.Runtime
	Collection collection.Collection
	Graph      graph.Graph
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
) (*Container, error) {
	g, err := graph.New(db, axArrow, os, m, resolveManifestFrom(axArrow, m))
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

	getArrow := func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		got, err := axArrow.Get(ctx, ns.String())
		if err != nil {
			return nil, err
		}
		return &got, nil
	}

	rt, err := runtime.New(
		getArrow,
		axRuntime,
		w,
		v,
		cat.MarkInstalled,
		func(ctx context.Context, ns domain.Namespace) (bool, error) {
			return g.HasDependents(ctx, ns, domain.Namespace(""))
		},
		func(ctx context.Context) ([]models.ArrowView, error) {
			return cat.List(ctx, nil)
		},
		os,
	)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: runtime: %w", err)
	}

	c := &Container{
		Arrow:      cat,
		Runtime:    rt,
		Collection: coll,
		Graph:      g,
	}

	if err := c.wireCallbacks(); err != nil {
		discardCollection(coll)
		return nil, err
	}

	return c, nil
}

// discardConstructionTimeout bounds the release of the collections database when
// New fails after opening it. Nothing is in flight at construction time, so this
// is a guard against a wedged drain rather than a budget anything should need.
const discardConstructionTimeout = 5 * time.Second

// discardCollection closes the collections database opened by NewFromDBPath when
// a later step of New fails, so a half-built container never leaves the file open
// with no owner.
func discardCollection(coll collection.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), discardConstructionTimeout)
	defer cancel()

	if err := coll.Shutdown(ctx); err != nil {
		slog.Warn("repositories: close collection store after failed construction", "err", err)
	}
}

// Shutdown drains every aggregate, blocking until in-flight commands have been
// persisted or ctx expires.
//
// Runtime drains first and Arrow last. When an install finishes, the runtime
// reaction's onEnd writes arrow.MarkInstalled and only then commits
// EndExecution (runtime/internal/hooks.go). Draining Arrow first would lose
// MarkInstalled while EndExecution still commits, leaving a ready runtime whose
// arrow carries no installed ref — nothing reconciles that. Draining Runtime
// first makes EndExecution the write that fails instead, so the runtime stays
// in `installing`, which RecoverTransients re-drives on the next boot
// (runtime/internal/recovery.go).
//
// Every phase runs even when an earlier one fails: a drain error must not leave
// the remaining aggregates accepting writes.
func (c *Container) Shutdown(ctx context.Context) error {
	var errs []error

	if err := c.Runtime.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("repositories: runtime shutdown: %w", err))
	}

	if err := c.Collection.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("repositories: collection shutdown: %w", err))
	}

	if err := c.Arrow.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("repositories: arrow shutdown: %w", err))
	}

	return errors.Join(errs...)
}

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

	if err := c.Arrow.OnArrowRemoved(func(ctx context.Context, ns domain.Namespace) error {
		if err := c.Graph.RemoveDependencies(ctx, ns); err != nil {
			return err
		}
		return c.Runtime.Forget(ctx, ns)
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
