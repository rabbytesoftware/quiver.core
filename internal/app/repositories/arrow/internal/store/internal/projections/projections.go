package projections

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Register subscribes to domain events and keeps the catalog storage in sync.
func Register(
	store storage.Store,
	axArrow asynx.Asynx[domain.Arrow],
	hub apphub.WebSocketHub,
) error {
	h := &handler{store: store}

	for _, topic := range []string{"arrow.added.*", "arrow.upgraded.*", "arrow.updated.*", "arrow.installed.*"} {
		t := topic
		if _, err := axArrow.Subscribe(asynx.Topic(t), func(
			ctx context.Context,
			evt asynxModels.Event[domain.Arrow],
		) {
			if err := h.aggregateAndSave(ctx, evt.Aggregate); err != nil {
				slog.ErrorContext(
					ctx,
					"catalog projection: "+t,
					"ns", evt.Aggregate.Namespace,
					"err", err,
				)
				return
			}

			if hub != nil {
				hub.BroadcastArrow(apphub.ArrowEvent{Kind: apphub.CatalogUpserted, Arrow: evt.Aggregate})
			}
		}); err != nil {
			return fmt.Errorf("catalog projection: subscribe %s: %w", t, err)
		}
	}

	if _, err := axArrow.OnForget(func(
		ctx context.Context,
		evt asynxModels.Event[domain.Arrow],
	) {
		if err := h.removeVersionAndCleanup(
			ctx,
			evt.Aggregate,
		); err != nil {
			slog.ErrorContext(ctx,
				"catalog projection: arrow forget",
				"ns", evt.Aggregate.Namespace,
				"err", err)
		}

		if hub != nil {
			hub.BroadcastArrow(apphub.ArrowEvent{Kind: apphub.CatalogRemoved, Arrow: evt.Aggregate})
		}
	}); err != nil {
		return fmt.Errorf("catalog projection: subscribe arrow forget: %w", err)
	}

	return nil
}

type handler struct {
	store storage.Store
}

// aggregateAndSave writes just the version the event carries. Aggregation into
// the parent row happens in SQL, so concurrent projections for different refs
// of one namespace cannot overwrite each other.
func (h *handler) aggregateAndSave(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	return h.store.SaveVersion(ctx, arrow.Namespace, arrow)
}

func (h *handler) removeVersionAndCleanup(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	bareNs := arrow.Namespace.BareNamespace()

	existing, err := h.store.FindByKey(ctx, bareNs.String())
	if err != nil {
		slog.WarnContext(
			ctx, "catalog projection: forget get failed",
			"ns", arrow.Namespace,
			"err", err,
		)
		return nil
	}
	if existing == nil {
		return nil
	}

	existing.Versions = removeVersion(
		existing.Versions,
		arrow.Namespace,
	)

	if len(existing.Versions) == 0 {
		return h.store.Delete(ctx, bareNs.String())
	}

	return h.store.Save(ctx, *existing)
}

func removeVersion(
	versions []storage.VersionRef,
	ns domain.Namespace,
) []storage.VersionRef {
	result := make([]storage.VersionRef, 0, len(versions))
	for _, v := range versions {
		if v.Namespace.String() != ns.String() {
			result = append(result, v)
		}
	}
	return result
}
