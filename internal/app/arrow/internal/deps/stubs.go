package deps

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (d *depsService) Execute(
	ctx context.Context,
	plan Plan,
) error {
	return fmt.Errorf("deps: Execute: not implemented")
}

func (d *depsService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return false, fmt.Errorf("deps: HasDependents: not implemented")
}

func (d *depsService) Orphans(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	return nil, fmt.Errorf("deps: Orphans: not implemented")
}
