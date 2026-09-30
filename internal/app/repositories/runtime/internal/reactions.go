package runtimeinternal

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

func RegisterReactions(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	hooks CatalogHooks,
	w wizardPkg.Wizard,
	tryAddDrain func(method string) (func(), bool),
) error {
	if _, err := axRuntime.Subscribe(asynx.Topic("runtime.begun.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		onBegun(ctx, evt, hooks, axRuntime, w, tryAddDrain)
	}); err != nil {
		return fmt.Errorf("runtime: runtime.begun subscription: %w", err)
	}

	return nil
}

func onBegun(
	ctx context.Context,
	evt asynxModels.Event[domainRuntime.ArrowRuntime],
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	tryAddDrain func(method string) (func(), bool),
) {
	rt := evt.Aggregate
	if rt.Execution == nil {
		return
	}
	if w == nil {
		return
	}

	request := wizardPkg.RunRequest{
		Namespace: rt.Ref,
		Method:    rt.Execution.Method,
		Steps:     stepsFromProgress(rt.Execution.Steps),
		Variables: rt.Execution.Variables,
		WorkDir:   rt.Execution.WorkDir,
		PID:       rt.Execution.PID,
	}
	exec := w.Start(context.WithoutCancel(ctx), request)

	done, ok := tryAddDrain(rt.Execution.Method)
	if !ok {
		return
	}
	go func() {
		defer done()
		superviseExecution(
			context.WithoutCancel(ctx),
			exec,
			request,
			rt.Execution.ID,
			hooks,
			w,
			axRuntime,
		)
	}()
}

func stepsFromProgress(steps []domainRuntime.StepProgress) []domainStep.Step {
	result := make([]domainStep.Step, len(steps))
	for i, sp := range steps {
		result[i] = sp.Step
	}
	return result
}
