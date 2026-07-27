package runtimeinternal

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

func RegisterReactions(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	markInstalled func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	markUninstalled func(ctx context.Context, ns domain.Namespace) error,
	w wizardPkg.Wizard,
	tryAddDrain func() (func(), bool),
) error {
	if _, err := axRuntime.Subscribe(asynx.Topic("runtime.begun.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		onBegun(ctx, evt, markInstalled, markUninstalled, axRuntime, w, tryAddDrain)
	}); err != nil {
		return fmt.Errorf("runtime: runtime.begun subscription: %w", err)
	}

	return nil
}

func onBegun(
	ctx context.Context,
	evt asynxModels.Event[domainRuntime.ArrowRuntime],
	markInstalled func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	markUninstalled func(ctx context.Context, ns domain.Namespace) error,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	tryAddDrain func() (func(), bool),
) {
	rt := evt.Aggregate
	if rt.Execution == nil {
		return
	}
	if w == nil {
		return
	}

	exec := w.Start(context.WithoutCancel(ctx), wizardPkg.RunRequest{
		Namespace: rt.Ref,
		Method:    rt.Execution.Method,
		Steps:     stepsFromProgress(rt.Execution.Steps),
		Variables: rt.Execution.Variables,
		WorkDir:   rt.Execution.WorkDir,
		PID:       rt.Execution.PID,
	})

	done, ok := tryAddDrain()
	if !ok {
		return
	}
	go func() {
		defer done()
		drainExecution(
			context.WithoutCancel(ctx),
			exec,
			rt.Ref.String(),
			rt.Execution.ID,
			rt.Execution.Method,
			markInstalled,
			markUninstalled,
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
