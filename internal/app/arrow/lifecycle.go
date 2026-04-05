package arrow

import (
	"context"

	"github.com/char2cs/asynx"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
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
