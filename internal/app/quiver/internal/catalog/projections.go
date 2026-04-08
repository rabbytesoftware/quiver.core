package catalog

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (c *catalogService) registerProjections() error {
	if _, err := c.axQuiver.Subscribe("quiver.added", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		_ = c.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axQuiver.Subscribe("quiver.updated", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		_ = c.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axQuiver.Subscribe("quiver.removed", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		_ = c.store.Delete(ctx, evt.Aggregate.Namespace)
	}); err != nil {
		return err
	}

	return nil
}
