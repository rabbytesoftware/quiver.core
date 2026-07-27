package projections

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Projector keeps the catalog read model in step with the arrow aggregate.
//
// It owns no subscription of its own. asynx runs one goroutine per subscriber,
// so a projector that subscribed here would race the reactions that make an
// arrow usable — its dependency edges above all. The arrow repository drives
// this instead, from the single subscriber that also runs those reactions, so
// the read-model write has a defined place in that order.
type Projector interface {
	Apply(
		ctx context.Context,
		arrow domain.Arrow,
	) error
	Forget(
		ctx context.Context,
		arrow domain.Arrow,
	) error
}

type projector struct {
	store storage.Store
}

func New(
	store storage.Store,
) Projector {
	return &projector{store: store}
}

// Apply writes just the version the event carries. Aggregation into the parent
// row happens in SQL, so concurrent projections for different refs of one
// namespace cannot overwrite each other.
func (p *projector) Apply(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	if err := p.store.SaveVersion(ctx, arrow.Namespace, arrow); err != nil {
		return fmt.Errorf("catalog projection: save version %s: %w", arrow.Namespace, err)
	}
	return nil
}

// Forget drops the version the tombstone carries and deletes the parent row
// once it was the last one, so a namespace never lingers with no versions.
func (p *projector) Forget(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	bareNs := arrow.Namespace.BareNamespace()

	existing, err := p.store.FindByKey(ctx, bareNs.String())
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
		if err := p.store.Delete(ctx, bareNs.String()); err != nil {
			return fmt.Errorf("catalog projection: delete %s: %w", bareNs, err)
		}
		return nil
	}

	if err := p.store.Save(ctx, *existing); err != nil {
		return fmt.Errorf("catalog projection: save %s: %w", bareNs, err)
	}
	return nil
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
