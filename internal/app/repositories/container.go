package repositories

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	repoarrow "github.com/rabbytesoftware/quiver/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/quiver"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

type Container struct {
	Arrow    repoarrow.Arrow
	Runtime  runtime.Runtime
	Quiver   quiver.Quiver
	Graph    graph.Graph
	Vault    vault.Vault
	Manifold manifold.Manifold
}

func New(
	db *gormdb.DB,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	axQuiver asynx.Asynx[domain.Quiver],
	quiverDBPath string,
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

	qv, err := quiver.NewFromDBPath(axQuiver, quiverDBPath, v, m)
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
		return nil, fmt.Errorf("repositories: runtime: %w", err)
	}

	c := &Container{
		Arrow:    cat,
		Runtime:  rt,
		Quiver:   qv,
		Graph:    g,
		Vault:    v,
		Manifold: m,
	}

	if err := c.wireCallbacks(); err != nil {
		return nil, err
	}

	return c, nil
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
		return c.Graph.RemoveDependencies(ctx, ns)
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

	if err := c.Quiver.OnQuiverFollowed(func(_ context.Context, q domain.Quiver) {
		hub.BroadcastQuiver(q)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnQuiverFollowed: %w", err)
	}

	if err := c.Quiver.OnQuiverUnfollowed(func(_ context.Context, ns domain.Namespace) {
		hub.BroadcastQuiver(domain.Quiver{Namespace: ns})
	}); err != nil {
		return fmt.Errorf("repositories: hub OnQuiverUnfollowed: %w", err)
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
