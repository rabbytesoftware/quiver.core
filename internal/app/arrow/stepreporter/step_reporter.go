package stepreporter

import (
	"context"

	"github.com/char2cs/asynx"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type StepReporter struct {
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	namespace    domain.Namespace
	indexOffset  int
}

func New(
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	namespace domain.Namespace,
	indexOffset int,
) *StepReporter {
	return &StepReporter{
		asynxRuntime: asynxRuntime,
		namespace:    namespace,
		indexOffset:  indexOffset,
	}
}

func (r *StepReporter) OnStepStarted(i int) {
	_ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i + r.indexOffset,
			ToStatus:  domainRuntime.StepStatusRunning,
		},
	)
}

func (r *StepReporter) OnStepCompleted(i int) {
	_ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i + r.indexOffset,
			ToStatus:  domainRuntime.StepStatusCompleted,
		},
	)
}

func (r *StepReporter) OnStepFailed(i int, err error) {
	errStr := err.Error()
	_ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i + r.indexOffset,
			ToStatus:  domainRuntime.StepStatusFailed,
			Error:     &errStr,
		},
	)
}
