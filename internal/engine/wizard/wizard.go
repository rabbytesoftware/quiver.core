package wizard

import (
	"context"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
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
		req RunRequest,
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

type dispatchFn = func(context.Context, wizstep.Request, domainstep.Step) error

type wizard struct {
	dispatch   map[domainstep.StepType]dispatchFn
	runtime    *runtime.Runtime
	executions sync.Map // nsKey -> *executionState
}

func New() (Wizard, error) {
	rt, err := runtime.New()
	if err != nil {
		return nil, err
	}

	w := &wizard{
		runtime:  rt,
		dispatch: make(map[domainstep.StepType]dispatchFn),
	}
	adapt(w.dispatch, domainstep.StepTypeRun, steprun.NewHandler(rt))
	adapt(w.dispatch, domainstep.StepTypeFetch, stepdownload.NewHandler())
	adapt(w.dispatch, domainstep.StepTypeSignal, stepsignal.NewHandler(rt))
	return w, nil
}

func (w *wizard) Shutdown(ctx context.Context) error {
	return w.runtime.Shutdown(ctx)
}

func (w *wizard) Execute(
	ctx context.Context,
	req RunRequest,
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
	req RunRequest,
	s domainstep.Step,
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

func adapt[S domainstep.Step](
	dispatch map[domainstep.StepType]dispatchFn,
	t domainstep.StepType,
	h wizstep.Handler[S],
) {
	dispatch[t] = func(
		ctx context.Context,
		req wizstep.Request,
		s domainstep.Step,
	) error {
		return h.Execute(ctx, req, s.(S))
	}
}
