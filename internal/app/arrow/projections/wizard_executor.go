package projections

import (
	"context"
	"errors"
	"iter"
	"slices"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// WizardExecutor is a projection that fires wizard.Execute in response to runtime.begun events.
type WizardExecutor struct {
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	asynxArrow   asynx.Asynx[domain.Arrow]
	wizard       wizard.Wizard
	svc          arrowService
}

func NewWizardExecutor(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	axArrow asynx.Asynx[domain.Arrow],
	wiz wizard.Wizard,
) *WizardExecutor {
	return &WizardExecutor{
		asynxRuntime: axRuntime,
		asynxArrow:   axArrow,
		wizard:       wiz,
	}
}

func (e *WizardExecutor) SetService(svc arrowService) {
	e.svc = svc
}

func (e *WizardExecutor) Handler() func(context.Context, asynxModels.Event[domainRuntime.ArrowRuntime]) {
	return func(ctx context.Context, evt asynxModels.Event[domainRuntime.ArrowRuntime]) {
		e.execute(ctx, evt.Aggregate)
	}
}

func (e *WizardExecutor) execute(ctx context.Context, rt domainRuntime.ArrowRuntime) {
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

	workDir, _ := e.svc.GetWorkDir(ctx, ns)

	reporter := stepreporter.New(e.asynxRuntime, ns)
	req := wizard.RunRequest{
		Namespace: ns,
		Variables: rt.ActiveRun.Variables,
		Steps:     steps,
		WorkDir:   workDir,
	}

	execErr := e.wizard.Execute(ctx, req, reporter)

	// executeSync already registered this namespace — it owns EndExecution.
	// Return silently without sending EndExecution.
	if errors.Is(execErr, wizard.ErrExecutionExists) {
		return
	}

	outcome := mapOutcome(execErr)
	_, _ = e.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
		Namespace: ns,
		Outcome:   outcome,
	})

	e.handlePostExecution(ctx, ns, method, execErr, outcome)
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

func (e *WizardExecutor) handlePostExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	execErr error,
	outcome domainRuntime.ExecutionOutcome,
) {
	switch method {
	case "_execute":
		if errors.Is(execErr, context.Canceled) && e.svc != nil {
			arrow, err := e.asynxArrow.Get(ctx, ns.String())
			if err == nil && len(arrow.Manifest.Lifecycle.Stop) > 0 {
				_ = e.svc.BeginExecution(ctx, ns, "_stop", nil)
			}
		}
	case "_uninstall":
		if outcome == domainRuntime.ExecutionOutcomeSuccess && e.svc != nil {
			_ = e.svc.CleanupAfterUninstall(ctx, ns)
		}
	}
}
