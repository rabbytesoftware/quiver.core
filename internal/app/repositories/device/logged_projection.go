package device

import (
	"context"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	devicestore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/device/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

type loggedProjection struct {
	projector devicestore.Projector
}

func (l loggedProjection) apply(
	ctx context.Context,
	evt asynxModels.Event[auth.Device],
) error {
	err := l.projector.Apply(ctx, evt.Aggregate)
	if err == nil {
		return nil
	}
	slog.ErrorContext(ctx, "device: projection failed", "device", evt.AggregateID, "err", err)
	return err
}
