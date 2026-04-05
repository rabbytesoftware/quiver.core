package projections

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// wizardCanceller is satisfied by wizard.Wizard without importing it.
type wizardCanceller interface {
	Cancel(ns domain.Namespace)
}

func StopCoordinator(wiz wizardCanceller) func(
	context.Context,
	asynxModels.Event[domainRuntime.ArrowRuntime],
) {
	return func(
		_ context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		if evt.EventName == "runtime.mark_stopping" {
			wiz.Cancel(evt.Aggregate.Namespace)
		}
	}
}
