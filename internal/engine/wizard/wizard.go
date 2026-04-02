package wizard

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
	stepdownload "github.com/rabbytesoftware/quiver/internal/engine/wizard/step/download"
	steprun "github.com/rabbytesoftware/quiver/internal/engine/wizard/step/run"
	stepsignal "github.com/rabbytesoftware/quiver/internal/engine/wizard/step/signal"
)

type Wizard interface {
	// Execute runs req.Steps sequentially. Returns:
	//   - ErrExecutionExists if a run is already active for req.Namespace
	//   - *StepError wrapping the handler error if a step with ExitOnFailure fails
	//   - nil if all steps complete (non-fatal failures are reported via reporter)
	Execute(
		ctx context.Context,
		req ExecutionRequest,
		reporter StepReporter,
	) error

	// Cancel aborts a running execution for the given namespace.
	// Attempts a graceful stop (SIGTERM, 5s timeout) before escalating to SIGKILL.
	// No-op if no execution is running.
	Cancel(
		namespace domain.Namespace,
	)

	// Shutdown stops all tracked OS processes and releases runtime resources.
	// Must be called when the wizard is no longer needed.
	Shutdown(ctx context.Context) error
}

type StepReporter interface {
	OnStepStarted(index int)
	OnStepCompleted(index int)
	OnStepFailed(index int, err error)
}

type wizard struct {
	handlers   []wizstep.Handler
	runtime    *runtime.Runtime
	executions sync.Map // nsKey -> *executionState
}

func New() (Wizard, error) {
	rt, err := runtime.New()
	if err != nil {
		return nil, err
	}

	w := &wizard{runtime: rt}
	w.handlers = []wizstep.Handler{
		steprun.NewHandler(rt),
		stepdownload.NewHandler(),
		stepsignal.NewHandler(rt),
	}
	return w, nil
}

func (w *wizard) Shutdown(ctx context.Context) error {
	return w.runtime.Shutdown(ctx)
}
