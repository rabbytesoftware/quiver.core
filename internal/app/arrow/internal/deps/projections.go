package deps

import (
	"context"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (d *depsService) registerProjections(
	axArrow asynx.Asynx[domain.Arrow],
) error {
	if _, err := axArrow.Subscribe("arrow.added", d.handleUpsert); err != nil {
		return err
	}

	if _, err := axArrow.Subscribe("arrow.updated", d.handleUpsert); err != nil {
		return err
	}

	if _, err := axArrow.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		d.handleRemove(ctx, evt)
	}); err != nil {
		return err
	}

	return nil
}

func (d *depsService) handleUpsert(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate
	for version, av := range arrow.Versions {
		edges := collectEdgesFromManifest(av)
		rows := edgesToRows(arrow.Namespace.BareNamespace().String(), version, edges)
		if err := d.fullStore.Save(ctx, arrow.Namespace.BareNamespace().String(), version, rows); err != nil {
			slog.WarnContext(ctx, "deps: save edges failed",
				"namespace", arrow.Namespace,
				"version", version,
				"err", err,
			)
		}
	}
}

func (d *depsService) handleRemove(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate
	for version := range arrow.Versions {
		if err := d.fullStore.DeleteFrom(ctx, arrow.Namespace.BareNamespace().String(), version); err != nil {
			slog.WarnContext(ctx, "deps: delete edges failed",
				"namespace", arrow.Namespace,
				"version", version,
				"err", err,
			)
		}
	}
}
