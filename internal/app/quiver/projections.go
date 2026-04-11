package quiver

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func registerWSProjections(
	axQuiver asynx.Asynx[domain.Quiver],
	hub apphub.WebSocketHub,
) error {
	if _, err := axQuiver.Subscribe("quiver.*", func(_ context.Context, evt asynxModels.Event[domain.Quiver]) {
		hub.BroadcastQuiver(evt.Aggregate)
	}); err != nil {
		return fmt.Errorf("ws quiver subscription: %w", err)
	}

	return nil
}
