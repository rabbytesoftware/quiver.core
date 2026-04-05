package projections

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func OnQuiverAdded(catalog quiverstore.QuiverCatalog) func(context.Context, asynxModels.Event[domain.Quiver]) {
	return func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		catalog.Save(ctx, evt.Aggregate)
	}
}

func OnQuiverUpdated(catalog quiverstore.QuiverCatalog) func(context.Context, asynxModels.Event[domain.Quiver]) {
	return func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		catalog.Save(ctx, evt.Aggregate)
	}
}

func OnQuiverRemoved(catalog quiverstore.QuiverCatalog) func(context.Context, asynxModels.Event[domain.Quiver]) {
	return func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		catalog.Delete(ctx, evt.Aggregate.Namespace)
	}
}
