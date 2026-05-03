package runtimeinternal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizardPkg "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func RegisterReactions(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	markInstalled func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error,
	w wizardPkg.Wizard,
	drainWg *sync.WaitGroup,
) error {
	if _, err := axRuntime.Subscribe(asynx.Topic("runtime.begun.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		onBegun(ctx, evt, markInstalled, axRuntime, w, drainWg)
	}); err != nil {
		return fmt.Errorf("runtime: runtime.begun subscription: %w", err)
	}

	return nil
}

func onBegun(
	ctx context.Context,
	evt asynxModels.Event[domainRuntime.ArrowRuntime],
	markInstalled func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	drainWg *sync.WaitGroup,
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

	drainWg.Add(1)
	go func() {
		defer drainWg.Done()
		drainExecution(
			context.WithoutCancel(ctx),
			exec,
			rt.Ref.String(),
			rt.Execution.Method,
			markInstalled,
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
