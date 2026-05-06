package projections

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver/internal/domain"
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
			if err := h.aggregateAndSave(
				ctx,
				evt.Aggregate,
			); err != nil {
				slog.ErrorContext(
					ctx,
					"catalog projection: "+t,
					"ns", evt.Aggregate.Namespace,
					"err", err,
				)
			}

			if hub != nil {
				hub.BroadcastArrow(evt.Aggregate)
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
			hub.BroadcastArrow(evt.Aggregate)
		}
	}); err != nil {
		return fmt.Errorf("catalog projection: subscribe arrow forget: %w", err)
	}

	return nil
}

type handler struct {
	store storage.Store
}

func (h *handler) aggregateAndSave(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	bareNs := arrow.Namespace.BareNamespace()

	existing, err := h.store.FindByKey(ctx, bareNs.String())
	if err != nil {
		return err
	}

	vm := storage.ViewModel{
		Namespace: bareNs,
		Metadata:  arrow,
		Versions:  []storage.VersionRef{},
	}
	if existing != nil {
		vm = *existing
	}

	vm.Versions = upsertVersion(vm.Versions, storage.VersionRef{
		Namespace: arrow.Namespace,
		Metadata:  arrow,
	})
	vm.Metadata = selectPreferredMetadata(vm.Versions)

	return h.store.Save(ctx, vm)
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

	existing.Metadata = selectPreferredMetadata(existing.Versions)
	return h.store.Save(ctx, *existing)
}

func upsertVersion(
	versions []storage.VersionRef,
	ver storage.VersionRef,
) []storage.VersionRef {
	for i, v := range versions {
		if v.Namespace.String() == ver.Namespace.String() {
			versions[i] = ver
			return versions
		}
	}
	return append(versions, ver)
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

func selectPreferredMetadata(
	versions []storage.VersionRef,
) domain.Arrow {
	if len(versions) == 0 {
		return domain.Arrow{}
	}
	for _, v := range versions {
		if v.Metadata.UserInstalled {
			return v.Metadata
		}
	}
	latest := versions[0]
	for _, v := range versions[1:] {
		if v.Metadata.InstalledAt.After(latest.Metadata.InstalledAt) {
			latest = v
		}
	}
	return latest.Metadata
}
