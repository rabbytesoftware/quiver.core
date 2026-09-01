package store

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Projector keeps the read model in step with the device aggregate.
type Projector interface {
	Apply(
		ctx context.Context,
		d auth.Device,
	) error
}

type projector struct {
	store Store
}

// NewProjector returns a Projector writing into store.
func NewProjector(
	store Store,
) Projector {
	return &projector{store: store}
}

func (p *projector) Apply(
	ctx context.Context,
	d auth.Device,
) error {
	if err := p.store.Upsert(ctx, d); err != nil {
		return fmt.Errorf("device projection: upsert %s: %w", d.ID, err)
	}

	return nil
}
