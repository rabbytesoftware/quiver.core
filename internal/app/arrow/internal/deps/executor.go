package deps

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (d *depsService) Execute(
	ctx context.Context,
	plan Plan,
) error {
	if len(plan) == 0 {
		return nil
	}

	var installed []PlanEntry

	for _, entry := range plan {
		if err := d.installSync(ctx, entry.Namespace); err != nil {
			d.rollback(ctx, installed)
			return err
		}

		installed = append(installed, entry)

		if entry.Type == domain.ServiceDep {
			_ = d.start(ctx, entry.Namespace)
		}
	}

	return nil
}

func (d *depsService) rollback(
	ctx context.Context,
	installed []PlanEntry,
) {
	for i := len(installed) - 1; i >= 0; i-- {
		_ = d.uninstallSync(ctx, installed[i].Namespace)
	}
}
