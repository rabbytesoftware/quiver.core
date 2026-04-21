package deps

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/char2cs/asynx"
	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
)

type Deps interface {
	Resolve(
		ctx context.Context,
		ns domain.Namespace,
	) (
		Plan,
		error,
	)

	Execute(
		ctx context.Context,
		plan Plan,
		triggeredBy domain.Namespace,
	) error

	Unplan(
		ctx context.Context,
		ns domain.Namespace,
	) (
		Plan,
		error,
	)

	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (
		bool,
		error,
	)

	Orphans(
		ctx context.Context,
		ns domain.Namespace,
	) (
		[]domain.Namespace,
		error,
	)

	DiffDeps(
		old *domain.Arrow,
		new *domain.Arrow,
	) DepDiff
}

type Plan []PlanEntry

type PlanEntry struct {
	Namespace domain.Namespace
	Type      domain.DepType
}

type ConstraintChange struct {
	Namespace     domain.Namespace
	OldConstraint string
	NewConstraint string
}

type DepDiff struct {
	Added       []domain.DependencyEdge
	Removed     []domain.DependencyEdge
	Constrained []ConstraintChange
}

type InstallSyncFunc func(
	ctx context.Context,
	ns domain.Namespace,
) error

type StartFunc func(
	ctx context.Context,
	ns domain.Namespace,
	triggeredBy domain.Namespace,
) error

type UninstallSyncFunc func(
	ctx context.Context,
	ns domain.Namespace,
) error

// IsInstalledFunc checks whether a dep has reached an installed state (ready/running).
// Returns false while the dep is absent or still installing.
type IsInstalledFunc func(ctx context.Context, ns domain.Namespace) bool

type depEdgeReader interface {
	HasAnyDependents(
		ctx context.Context,
		toNs, excludeFromNs string,
	) (bool, error)

	ByDependency(
		ctx context.Context,
		toNs, toVersion string,
	) ([]depsstore.DepEdgeRow, error)
}

type depsService struct {
	os              domain.OS
	manifold        manifold.Manifold
	depTree         deptree.DepTree
	resolveManifest manifest.ResolveFunc
	reader          depEdgeReader
	store           depsstore.DepEdgeStore
	installSync     InstallSyncFunc
	start           StartFunc
	uninstallSync   UninstallSyncFunc
	isInstalled     IsInstalledFunc
}

func New(
	os domain.OS,
	axArrow asynx.Asynx[domain.Arrow],
	m manifold.Manifold,
	dt deptree.DepTree,
	resolve manifest.ResolveFunc,
	st depsstore.DepEdgeStore,
	install InstallSyncFunc,
	startFn StartFunc,
	uninstall UninstallSyncFunc,
	isInstalled IsInstalledFunc,
) (Deps, error) {
	d := &depsService{
		os:              os,
		manifold:        m,
		depTree:         dt,
		resolveManifest: resolve,
		reader:          st,
		store:           st,
		installSync:     install,
		start:           startFn,
		uninstallSync:   uninstall,
		isInstalled:     isInstalled,
	}

	if err := d.registerProjections(axArrow); err != nil {
		return nil, fmt.Errorf("deps: register projections: %w", err)
	}

	return d, nil
}

func NewTestable(
	os domain.OS,
	dt deptree.DepTree,
	resolve manifest.ResolveFunc,
	st depEdgeReader,
	install InstallSyncFunc,
	startFn StartFunc,
	uninstall UninstallSyncFunc,
	isInstalled IsInstalledFunc,
) Deps {
	return &depsService{
		os:              os,
		depTree:         dt,
		resolveManifest: resolve,
		reader:          st,
		store:           nil,
		installSync:     install,
		start:           startFn,
		uninstallSync:   uninstall,
		isInstalled:     isInstalled,
	}
}

func (d *depsService) Resolve(
	ctx context.Context,
	ns domain.Namespace,
) (Plan, error) {
	typeIndex := make(map[domain.Namespace]domain.DepType)

	resolver := func(
		ctx context.Context,
		depNs domain.Namespace,
	) ([]domain.Namespace, error) {
		m, err := d.resolveManifest(ctx, depNs)
		if err != nil {
			return nil, err
		}

		// Use current OS target only — no cross-OS pollution.
		target, ok := m.Targets[d.os]
		if !ok {
			return nil, nil
		}

		var children []domain.Namespace
		for _, edge := range target.Tools {
			resolved, resolveErr := d.resolveEdgeNs(ctx, edge)
			if resolveErr != nil {
				return nil, resolveErr
			}
			typeIndex[resolved.BareNamespace()] = domain.ToolDep
			children = append(children, resolved)
		}

		for _, edge := range target.Services {
			resolved, resolveErr := d.resolveEdgeNs(ctx, edge)
			if resolveErr != nil {
				return nil, resolveErr
			}
			typeIndex[resolved.BareNamespace()] = domain.ServiceDep
			children = append(children, resolved)
		}

		return dedupNamespaces(children), nil
	}

	ordered, err := d.depTree.Resolve(ctx, ns, resolver)
	if err != nil {
		return nil, err
	}

	rootBare := ns.BareNamespace()
	var plan Plan
	for _, dep := range ordered {
		if dep.BareNamespace() == rootBare {
			continue
		}
		depType, ok := typeIndex[dep.BareNamespace()]
		if !ok {
			depType = domain.ToolDep
		}
		plan = append(plan, PlanEntry{
			Namespace: dep,
			Type:      depType,
		})
	}

	return plan, nil
}

func (d *depsService) Unplan(
	ctx context.Context,
	ns domain.Namespace,
) (Plan, error) {
	plan, err := d.Resolve(ctx, ns)
	if err != nil {
		return nil, err
	}

	slices.Reverse(plan)

	return plan, nil
}

func (d *depsService) DiffDeps(
	old *domain.Arrow,
	new *domain.Arrow,
) DepDiff {
	oldEdges := collectEdgesForOS(old, d.os)
	newEdges := collectEdgesForOS(new, d.os)

	oldByBare := make(map[domain.Namespace]domain.DependencyEdge, len(oldEdges))
	for _, e := range oldEdges {
		oldByBare[e.Namespace.BareNamespace()] = e
	}

	newByBare := make(map[domain.Namespace]domain.DependencyEdge, len(newEdges))
	for _, e := range newEdges {
		newByBare[e.Namespace.BareNamespace()] = e
	}

	var added []domain.DependencyEdge
	for bare, e := range newByBare {
		if _, exists := oldByBare[bare]; !exists {
			added = append(added, e)
		}
	}

	var removed []domain.DependencyEdge
	for bare, e := range oldByBare {
		if _, exists := newByBare[bare]; !exists {
			removed = append(removed, e)
		}
	}

	var constrained []ConstraintChange
	for bare, oldEdge := range oldByBare {
		newEdge, exists := newByBare[bare]
		if !exists {
			continue
		}
		if oldEdge.Constraint == newEdge.Constraint {
			continue
		}
		constrained = append(constrained, ConstraintChange{
			Namespace:     bare,
			OldConstraint: oldEdge.Constraint,
			NewConstraint: newEdge.Constraint,
		})
	}

	return DepDiff{
		Added:       added,
		Removed:     removed,
		Constrained: constrained,
	}
}

func (d *depsService) Execute(
	ctx context.Context,
	plan Plan,
	triggeredBy domain.Namespace,
) error {
	if len(plan) == 0 {
		return nil
	}

	var installed []PlanEntry

	for _, entry := range plan {
		if err := d.installSync(ctx, entry.Namespace); err != nil {
			if !errors.Is(err, apperrors.ErrStateViolation) {
				d.rollback(ctx, installed)
				return err
			}
			// Dep is being installed concurrently — wait for it to finish.
			if waitErr := d.waitInstalled(ctx, entry.Namespace); waitErr != nil {
				d.rollback(ctx, installed)
				return waitErr
			}
		}
		installed = append(installed, entry)

		if entry.Type == domain.ServiceDep {
			if err := d.start(ctx, entry.Namespace, triggeredBy); err != nil {
				if !errors.Is(err, apperrors.ErrStateViolation) {
					d.rollback(ctx, installed)
					return fmt.Errorf("deps: start service %s: %w", entry.Namespace, err)
				}
				// Service dep still installing — wait until ready, then retry.
				if waitErr := d.waitInstalled(ctx, entry.Namespace); waitErr != nil {
					d.rollback(ctx, installed)
					return fmt.Errorf("deps: wait service dep %s: %w", entry.Namespace, waitErr)
				}
				if retryErr := d.start(ctx, entry.Namespace, triggeredBy); retryErr != nil &&
					!errors.Is(retryErr, apperrors.ErrStateViolation) {
					d.rollback(ctx, installed)
					return fmt.Errorf("deps: start service dep %s: %w", entry.Namespace, retryErr)
				}
			}
		}
	}
	return nil
}

// resolveEdgeNs resolves a dep edge to a concrete namespace@ref.
// Glob constraints are resolved via the manifold; exact refs are used directly.
func (d *depsService) resolveEdgeNs(
	ctx context.Context,
	edge domain.DependencyEdge,
) (domain.Namespace, error) {
	ref := edge.Constraint
	if ref == "" {
		return edge.Namespace.BareNamespace(), nil
	}
	if containsGlob(ref) {
		if d.manifold == nil {
			return edge.Namespace.BareNamespace().WithRef(ref), nil
		}
		resolved, err := d.manifold.ResolveConstraint(ctx, edge.Namespace, ref)
		if err != nil {
			return "", fmt.Errorf("deps: resolve constraint %q for %s: %w",
				ref, edge.Namespace, err)
		}
		return edge.Namespace.BareNamespace().WithRef(resolved), nil
	}
	return edge.Namespace.BareNamespace().WithRef(ref), nil
}

func containsGlob(
	s string,
) bool {
	return strings.ContainsAny(s, "*?[")
}

// waitInstalled polls until the dep reaches an installed state (ready/running)
// or the context is cancelled. Used to wait out concurrent dep installations.
func (d *depsService) waitInstalled(ctx context.Context, ns domain.Namespace) error {
	for {
		if d.isInstalled(ctx, ns) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("deps: context cancelled waiting for %s to install", ns)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (d *depsService) rollback(
	ctx context.Context,
	installed []PlanEntry,
) {
	for i := len(installed) - 1; i >= 0; i-- {
		_ = d.uninstallSync(ctx, installed[i].Namespace)
	}
}

func (d *depsService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return d.reader.HasAnyDependents(
		ctx,
		ns.BareNamespace().String(),
		excludeNs.BareNamespace().String(),
	)
}

func (d *depsService) Orphans(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	plan, err := d.Resolve(ctx, ns)
	if err != nil {
		return nil, err
	}

	var orphans []domain.Namespace
	for _, entry := range plan {
		hasDeps, depErr := d.HasDependents(ctx, entry.Namespace, ns)
		if depErr != nil || hasDeps {
			continue
		}
		orphans = append(orphans, entry.Namespace)
	}

	return orphans, nil
}
