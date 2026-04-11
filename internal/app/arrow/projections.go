package arrow

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

func registerWSProjections(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	hub apphub.WebSocketHub,
) error {
	if _, err := axArrow.Subscribe("arrow.*", func(_ context.Context, evt asynxModels.Event[domain.Arrow]) {
		hub.BroadcastArrow(evt.Aggregate)
	}); err != nil {
		return fmt.Errorf("ws arrow subscription: %w", err)
	}

	if _, err := axRuntime.Subscribe("runtime.*", func(_ context.Context, evt asynxModels.Event[domainRuntime.ArrowRuntime]) {
		hub.BroadcastArrowRuntime(evt.Aggregate)
	}); err != nil {
		return fmt.Errorf("ws runtime subscription: %w", err)
	}

	return nil
}
