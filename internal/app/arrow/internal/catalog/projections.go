package catalog

import (
	"context"
	"errors"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (c *catalogService) registerProjections() error {
	if _, err := c.axArrow.Subscribe("arrow.added", func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		_ = c.aggregateAndSave(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axArrow.Subscribe("arrow.updated", func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		_ = c.aggregateAndSave(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axArrow.Subscribe("arrow.installed", func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		_ = c.aggregateAndSave(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := c.axArrow.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		_ = c.removeVersionAndCleanup(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	return nil
}

func (c *catalogService) aggregateAndSave(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	bareNs := arrow.Namespace.BareNamespace()

	existing, getErr := c.store.Get(ctx, bareNs)
	if getErr != nil && !errors.Is(getErr, asynxModels.ErrNotFound) {
		return getErr
	}

	var vm store.ArrowViewModel
	if existing == nil {
		vm = store.ArrowViewModel{
			Namespace: bareNs,
			Metadata:  arrow,
			Versions:  []store.ArrowVersionRef{},
		}
	} else {
		vm = *existing
	}

	vm.Versions = upsertVersion(vm.Versions, store.ArrowVersionRef{
		Namespace: arrow.Namespace,
		Metadata:  arrow,
	})

	vm.Metadata = selectPreferredMetadata(vm.Versions)

	return c.store.Save(ctx, vm)
}

func (c *catalogService) removeVersionAndCleanup(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	bareNs := arrow.Namespace.BareNamespace()

	existing, getErr := c.store.Get(ctx, bareNs)
	if getErr != nil && !errors.Is(getErr, asynxModels.ErrNotFound) {
		slog.WarnContext(ctx, "forget: arrow catalog get failed", "namespace", arrow.Namespace, "err", getErr)
		return nil
	}

	if existing == nil {
		return nil
	}

	vm := *existing
	vm.Versions = removeVersion(vm.Versions, arrow.Namespace)

	if len(vm.Versions) == 0 {
		return c.store.Delete(ctx, bareNs)
	}

	vm.Metadata = selectPreferredMetadata(vm.Versions)
	return c.store.Save(ctx, vm)
}

// upsertVersion adds or replaces a version by namespace in the versions list.
func upsertVersion(versions []store.ArrowVersionRef, ver store.ArrowVersionRef) []store.ArrowVersionRef {
	for i, v := range versions {
		if v.Namespace.String() == ver.Namespace.String() {
			versions[i] = ver
			return versions
		}
	}
	return append(versions, ver)
}

// removeVersion removes a version by namespace from the versions list.
func removeVersion(versions []store.ArrowVersionRef, ns domain.Namespace) []store.ArrowVersionRef {
	result := make([]store.ArrowVersionRef, 0, len(versions))
	for _, v := range versions {
		if v.Namespace.String() != ns.String() {
			result = append(result, v)
		}
	}
	return result
}

// selectPreferredMetadata returns the metadata for the preferred version:
// prefer UserInstalled == true; otherwise pick latest by InstalledAt.
func selectPreferredMetadata(versions []store.ArrowVersionRef) domain.Arrow {
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
