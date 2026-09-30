package deps

import (
	"context"
	"errors"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
)

// SyncTargetDeps syncs whatever is pending, so a retried update that finds
// its manifest already staged syncs what the first attempt left pending
// rather than a diff that is now empty.
func (d *deps) SyncTargetDeps(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	target *domain.Arrow,
) error {
	diff := d.graph.DiffDeps(current, target)
	if len(diff.Added) > 0 || len(diff.Removed) > 0 {
		if err := d.runtime.MarkOutdated(ctx, ns, edgesToNs(diff.Added), edgesToNs(diff.Removed)); err != nil {
			return fmt.Errorf("mark outdated: %w", err)
		}
	}

	state, err := d.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if state != domain.ArrowStateOutdated {
		return nil
	}
	return d.syncDeps(ctx, ns)
}

func (d *deps) syncDeps(
	ctx context.Context,
	ns domain.Namespace,
) error {
	rt, err := d.runtime.GetRuntime(ctx, ns)
	if err != nil {
		return fmt.Errorf("sync deps: %w", err)
	}
	if rt == nil || rt.State != domain.ArrowStateOutdated {
		return fmt.Errorf("sync deps: %w", apperrors.ErrStateViolation)
	}

	syncInfo := rt.PendingDepSync
	if syncInfo == nil {
		return nil
	}

	identities, err := d.ensureAddedDeps(ctx, ns, syncInfo.AddedDeps)
	if err != nil {
		return err
	}
	plan, err := d.graph.Resolve(ctx, ns)
	if errors.Is(err, deptree.ErrCyclicDependency) {
		return fmt.Errorf("sync deps: %w: %w", apperrors.ErrInvalidManifest, err)
	}

	for _, depNs := range syncInfo.AddedDeps {
		if err := d.installOneDep(ctx, identities[depNs]); err != nil {
			return err
		}
	}

	if err == nil { //nolint:nestif
		planMap := make(map[domain.Namespace]domain.DepType, len(plan))
		for _, entry := range plan {
			planMap[entry.Namespace] = entry.Type
		}
		for _, depNs := range syncInfo.AddedDeps {
			if planMap[depNs] == domain.ServiceDep {
				if startErr := d.startServiceDep(ctx, identities[depNs]); startErr != nil {
					return fmt.Errorf("sync deps: start service dep %s: %w", depNs, startErr)
				}
			}
		}
	}

	for _, declared := range syncInfo.RemovedDeps {
		d.retireRemovedDep(ctx, declared)
	}

	return nil
}

// ensureAddedDeps catalogues the dependencies a row gained and returns the
// identity each declaration installs. A row that gained itself is refused.
func (d *deps) ensureAddedDeps(
	ctx context.Context,
	ns domain.Namespace,
	added []domain.Namespace,
) (map[domain.Namespace]domain.Namespace, error) {
	identities := make(map[domain.Namespace]domain.Namespace, len(added))
	for _, depNs := range added {
		identity, err := d.ensureDependency(ctx, depNs)
		if err != nil {
			return nil, fmt.Errorf("sync deps: %w", err)
		}
		if identity == ns {
			return nil, fmt.Errorf("sync deps: %s depends on itself: %w", ns, apperrors.ErrInvalidManifest)
		}
		identities[depNs] = identity
	}
	return identities, nil
}

// retireRemovedDep uninstalls, or stops, a dependency a row no longer
// declares once nothing else needs it. The declaration is mapped onto the
// row the catalog holds it under, since it may spell a commit in another case.
func (d *deps) retireRemovedDep(
	ctx context.Context,
	declared domain.Namespace,
) {
	depNs, err := d.arrow.ResolveCatalogued(ctx, declared)
	if err != nil {
		return
	}
	arrow, getErr := d.arrow.Get(ctx, depNs)
	if getErr != nil || arrow == nil || arrow.UserInstalled {
		return
	}
	hasDeps, depsErr := d.graph.HasDependents(ctx, depNs, "")
	if depsErr != nil || hasDeps {
		return
	}
	depState, stateErr := d.runtime.GetState(ctx, depNs)
	if stateErr != nil {
		return
	}
	switch depState {
	case domain.ArrowStateReady, domain.ArrowStateOutdated:
		_ = d.runtime.BeginUninstall(ctx, depNs, nil)
	case domain.ArrowStateRunning, domain.ArrowStateStopping:
		_ = d.runtime.BeginStop(ctx, depNs)
	case domain.ArrowStateAbsent,
		domain.ArrowStateInstalling,
		domain.ArrowStateUpdating,
		domain.ArrowStateDraining,
		domain.ArrowStateDetached,
		domain.ArrowStateUninstalling,
		domain.ArrowStateRemoved:
	}
}

func edgesToNs(edges []domain.DependencyEdge) []domain.Namespace {
	ns := make([]domain.Namespace, 0, len(edges))
	for _, e := range edges {
		ns = append(ns, e.Namespace)
	}
	return ns
}
