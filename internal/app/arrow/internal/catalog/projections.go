package catalog

import (
	"context"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (c *catalogService) registerProjections() error {
	if _, err := c.axArrow.Subscribe("arrow.added", func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		_ = c.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axArrow.Subscribe("arrow.updated", func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		_ = c.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axArrow.Subscribe("arrow.installed", func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		_ = c.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axArrow.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		if err := c.store.Delete(ctx, evt.Aggregate.Namespace); err != nil {
			slog.WarnContext(ctx, "forget: arrow catalog delete failed", "namespace", evt.Aggregate.Namespace, "err", err)
		}
	}); err != nil {
		return err
	}

	return nil
}
