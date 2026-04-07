package runner

import (
	"context"

	"github.com/char2cs/asynx"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type StepReporter struct {
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	namespace    domain.Namespace
}

func New(
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	namespace domain.Namespace,
) *StepReporter {
	return &StepReporter{
		asynxRuntime: asynxRuntime,
		namespace:    namespace,
	}
}

func (r *StepReporter) OnStepStarted(i int) {
	_, _ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i,
			ToStatus:  domainRuntime.StepStatusRunning,
		},
	)
}

func (r *StepReporter) OnStepCompleted(i int) {
	_, _ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i,
			ToStatus:  domainRuntime.StepStatusCompleted,
		},
	)
}

func (r *StepReporter) OnStepFailed(i int, err error) {
	errStr := err.Error()
	_, _ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i,
			ToStatus:  domainRuntime.StepStatusFailed,
			Error:     &errStr,
		},
	)
}
