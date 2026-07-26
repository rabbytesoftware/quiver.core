package projections

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// AggregateAndSave exposes the unexported aggregateAndSave for white-box tests.
func AggregateAndSave(
	store storage.Store,
	ctx context.Context,
	arrow domain.Arrow,
) error {
	h := &handler{store: store}
	return h.aggregateAndSave(ctx, arrow)
}

// RemoveVersionAndCleanup exposes removeVersionAndCleanup for white-box tests.
func RemoveVersionAndCleanup(
	store storage.Store,
	ctx context.Context,
	arrow domain.Arrow,
) error {
	h := &handler{store: store}
	return h.removeVersionAndCleanup(ctx, arrow)
}
