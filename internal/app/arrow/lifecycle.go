package arrow

import (
	"context"

	"github.com/char2cs/asynx"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

func hasDependentsOf(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
	catalog arrowstore.ArrowCatalog,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
) (bool, error) {
	arrows, err := catalog.List(ctx)
	if err != nil {
		return false, err
	}

	for _, arrow := range arrows {
		if arrow.Removed || arrow.Namespace == excludeNs || arrow.Namespace == ns {
			continue
		}

		rt, err := axRuntime.Get(ctx, arrow.Namespace.String())
		if err != nil || rt.State == domain.ArrowStateAbsent || rt.Namespace == "" {
			continue
		}

		entry, _, err := v.GetArrow(ctx, arrow.Namespace)
		if err != nil {
			continue
		}

		for _, dep := range entry.Manifest.Dependencies {
			if dep == ns {
				return true, nil
			}
		}
		for _, dep := range entry.IndirectDependencies {
			if dep == ns {
				return true, nil
			}
		}
	}

	return false, nil
}

func (svc *arrowService) hasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return hasDependentsOf(ctx, ns, excludeNs, svc.catalog, svc.asynxRuntime, svc.engines.Vault)
}

func (svc *arrowService) cleanupAfterUninstall(
	ctx context.Context,
	ns domain.Namespace,
) {
	entry, _, err := svc.engines.Vault.GetArrow(ctx, ns)
	if err != nil {
		_ = svc.engines.Vault.DeleteArrow(ctx, ns)
		return
	}

	allDeps := make([]domain.Namespace, 0, len(entry.Manifest.Dependencies)+len(entry.IndirectDependencies))
	allDeps = append(allDeps, entry.Manifest.Dependencies...)
	allDeps = append(allDeps, entry.IndirectDependencies...)

	orphaned := make(map[domain.Namespace]bool)
	for _, dep := range allDeps {
		hasDeps, depErr := svc.hasDependents(ctx, dep, ns)
		if depErr != nil || hasDeps {
			continue
		}
		orphaned[dep] = true
	}

	vaultResolver := func(resolveCtx context.Context, depNs domain.Namespace) ([]domain.Namespace, error) {
		depEntry, _, depErr := svc.engines.Vault.GetArrow(resolveCtx, depNs)
		if depErr != nil || depEntry == nil {
			return nil, nil
		}
		return depEntry.Manifest.Dependencies, nil
	}

	topoOrder, topoErr := svc.engines.DepTree.Resolve(ctx, ns, deptree.ResolverFunc(vaultResolver))
	if topoErr != nil {
		_ = svc.engines.Vault.DeleteArrow(ctx, ns)
		return
	}

	for i := len(topoOrder) - 1; i >= 0; i-- {
		dep := topoOrder[i]
		if dep == ns || !orphaned[dep] {
			continue
		}
		if uninstallErr := svc.executeSync(ctx, dep, "_uninstall", nil); uninstallErr != nil {
			continue
		}
		_ = svc.engines.Vault.DeleteArrow(ctx, dep)
	}

	_ = svc.engines.Vault.DeleteArrow(ctx, ns)
}

func (svc *arrowService) CleanupAfterUninstall(
	ctx context.Context,
	ns domain.Namespace,
) error {
	svc.cleanupAfterUninstall(ctx, ns)
	return nil
}
