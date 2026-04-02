package wizard

import (
	"context"
	goruntime "runtime"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

func (w *wizard) Execute(
	ctx context.Context,
	req ExecutionRequest,
	reporter StepReporter,
) error {
	nsKey := req.Namespace.String()
	execCtx, cancel := context.WithCancel(ctx)
	state := newExecutionState(cancel)

	_, loaded := w.executions.LoadOrStore(nsKey, state)
	if loaded {
		cancel()
		return ErrExecutionExists
	}
	defer w.executions.Delete(nsKey)
	defer w.runtime.CleanupFinished()
	defer cancel()

	for i, s := range req.Steps {
		reporter.OnStepStarted(i)

		if err := w.executeStep(execCtx, state, req, s); err != nil {
			reporter.OnStepFailed(i, err)
			if s.ExitOnFailure() {
				return &StepError{Index: i, Step: s, Cause: err}
			}
			continue
		}

		reporter.OnStepCompleted(i)
	}

	return nil
}

func (w *wizard) Cancel(
	namespace domain.Namespace,
) {
	nsKey := namespace.String()

	val, ok := w.executions.Load(nsKey)
	if !ok {
		return
	}
	state, ok := val.(*executionState)
	if !ok {
		return
	}

	state.cancel()

	key, ok := state.GetKey()
	if !ok {
		return
	}
	proc, err := w.runtime.GetByKey(key)
	if err != nil {
		return
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := proc.Stop(stopCtx); err != nil {
		_ = proc.Kill(context.Background())
	}
}

func (w *wizard) executeStep(
	ctx context.Context,
	state *executionState,
	req ExecutionRequest,
	s step.Step,
) error {
	stepReq := wizstep.Request{
		NSKey:   req.Namespace.String(),
		WorkDir: req.WorkDir,
		Vars:    req.Variables,
		OSArch:  domain.OS(goruntime.GOOS + "/" + goruntime.GOARCH),
		Tracker: state,
	}
	fn, ok := w.dispatch[s.Type()]
	if !ok {
		return ErrUnknownStepType
	}
	return fn(ctx, stepReq, s)
}
