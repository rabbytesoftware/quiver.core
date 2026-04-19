package deps

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (d *depsService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return d.store.HasAnyDependents(
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
		orphans = append(orphans, entry.Namespace.BareNamespace())
	}

	return orphans, nil
}
