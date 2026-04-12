package runner

import (
	"context"
	"errors"
	"iter"
	"path/filepath"
	"slices"

	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func (r *runnerService) registerProjections() error {
	if _, err := r.axRuntime.Subscribe("runtime.begun", r.handleExecution); err != nil {
		return err
	}
	if _, err := r.axRuntime.Subscribe("runtime.mark_stopping", r.handleStopSignal); err != nil {
		return err
	}
	return nil
}

func (r *runnerService) handleExecution(
	ctx context.Context,
	evt asynxModels.Event[domainRuntime.ArrowRuntime],
) {
	r.execute(ctx, evt.Aggregate)
}

func (r *runnerService) handleStopSignal(
	_ context.Context,
	evt asynxModels.Event[domainRuntime.ArrowRuntime],
) {
	if r.wizard != nil {
		r.wizard.Cancel(evt.Aggregate.Namespace)
	}
}

func (r *runnerService) execute(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
) {
	if rt.ActiveRun == nil {
		return
	}

	ns := rt.Namespace
	method := rt.ActiveRun.Method

	steps := slices.Collect(iter.Seq[domainstep.Step](func(yield func(domainstep.Step) bool) {
		for _, sp := range rt.ActiveRun.Steps {
			if !yield(sp.Step) {
				return
			}
		}
	}))

	workDir := ""
	if entry, homePath, err := r.vault.GetArrow(ctx, ns); err == nil && entry != nil {
		workDir = filepath.Dir(homePath)
	}

	reporter := NewStepReporter(r.axRuntime, ns)
	req := wizard.RunRequest{
		Namespace: ns,
		Variables: rt.ActiveRun.Variables,
		Steps:     steps,
		WorkDir:   workDir,
	}

	var execErr error
	if r.wizard != nil {
		execErr = r.wizard.Execute(ctx, req, reporter)
	}

	outcome := mapOutcome(execErr)
	_, _ = r.axRuntime.Send(ctx, arrowcmds.EndExecution{
		Namespace: ns,
		Outcome:   outcome,
	})

	if r.hook != nil {
		r.hook(ctx, ns, method, execErr, outcome)
	}
}

func mapOutcome(err error) domainRuntime.ExecutionOutcome {
	if err == nil {
		return domainRuntime.ExecutionOutcomeSuccess
	}
	if errors.Is(err, context.Canceled) {
		return domainRuntime.ExecutionOutcomeCancelled
	}
	return domainRuntime.ExecutionOutcomeFailed
}
