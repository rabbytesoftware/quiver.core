package projections

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func OnArrowAdded(
	catalog arrowstore.ArrowCatalog,
) func(context.Context, asynxModels.Event[domain.Arrow]) {
	return func(
		ctx context.Context,
		evt asynxModels.Event[domain.Arrow],
	) {
		catalog.Save(ctx, evt.Aggregate)
	}
}

func OnArrowUpdated(
	catalog arrowstore.ArrowCatalog,
) func(context.Context, asynxModels.Event[domain.Arrow]) {
	return func(
		ctx context.Context,
		evt asynxModels.Event[domain.Arrow],
	) {
		catalog.Save(ctx, evt.Aggregate)
	}
}

func OnArrowRemoved(
	catalog arrowstore.ArrowCatalog,
) func(context.Context, asynxModels.Event[domain.Arrow]) {
	return func(
		ctx context.Context,
		evt asynxModels.Event[domain.Arrow],
	) {
		catalog.Delete(ctx, evt.Aggregate.Namespace)
	}
}
