package wizard

import (
	"context"
	"fmt"
	goruntime "runtime"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/models"
	wizrt "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step"
	stepdeps "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step/dependencies"
	stepdownload "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step/download"
	steprun "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step/run"
	stepsignal "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step/signal"
)

// Re-exports from internal/models — public API of the wizard package.
type (
	Event      = models.Event
	EventKind  = models.EventKind
	Execution  = models.Execution
	RunRequest = models.RunRequest
)

const (
	EventKindStepStarted   = models.EventKindStepStarted
	EventKindStepCompleted = models.EventKindStepCompleted
	EventKindStepFailed    = models.EventKindStepFailed
	EventKindPID           = models.EventKindPID
	EventKindEnded         = models.EventKindEnded
)

var ErrUnknownStepType = models.ErrUnknownStepType

type Wizard interface {
	// Start executes req.Steps in a background goroutine and returns immediately.
	// Events are delivered on the returned Execution; the goroutine exits when all
	// steps finish or ctx is cancelled.
	Start(
		ctx context.Context,
		req RunRequest,
	) Execution

	// Shutdown cancels all active executions and waits for their goroutines to exit.
	// If ctx expires before all goroutines finish, Shutdown returns ctx.Err() but
	// cleanup continues in the background.
	Shutdown(
		ctx context.Context,
	) error

	// ProcessAlive reports whether the process with the given PID is still running.
	ProcessAlive(pid int) bool
}

// DispatchFn is the untyped handler signature used in the dispatch table.
type DispatchFn = func(context.Context, wizstep.Request, domainstep.Step) error

type wizard struct {
	dispatch     map[domainstep.StepType]DispatchFn
	runtime      wizrt.Runtime
	shutdownCtx  context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	shutdownOnce sync.Once
	done         chan struct{}
}

// depExec is the app-layer function that resolves DependenciesSteps;
// pass nil to treat dependency steps as no-ops.
func New(
	depExec stepdeps.Executor,
) (Wizard, error) {
	rt, err := wizrt.New()
	if err != nil {
		return nil, err
	}

	shutdownCtx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- cancel is stored in w.cancel and called in Shutdown

	w := &wizard{
		runtime:     rt,
		shutdownCtx: shutdownCtx,
		cancel:      cancel,
		dispatch:    make(map[domainstep.StepType]DispatchFn),
		done:        make(chan struct{}),
	}

	adapt(w.dispatch, domainstep.StepTypeRun, steprun.NewHandler(rt))
	adapt(w.dispatch, domainstep.StepTypeFetch, stepdownload.NewHandler())
	adapt(w.dispatch, domainstep.StepTypeSignal, stepsignal.NewHandler(rt))
	adapt(w.dispatch, domainstep.StepTypeDependencies, stepdeps.NewHandler(depExec))

	return w, nil
}

func (w *wizard) Start(
	ctx context.Context,
	req RunRequest,
) Execution {
	exec := models.NewExecution()

	w.wg.Go(func() {
		runCtx, runCancel := context.WithCancel(ctx)
		stop := context.AfterFunc(w.shutdownCtx, runCancel)
		defer stop()
		defer runCancel()

		outcome := w.runSteps(runCtx, req, exec)
		exec.Finish(outcome)
	})

	return exec
}

func (w *wizard) ProcessAlive(pid int) bool {
	return w.runtime.ProcessAlive(pid)
}

func (w *wizard) Shutdown(
	ctx context.Context,
) error {
	w.shutdownOnce.Do(func() {
		w.cancel()
		go func() {
			w.wg.Wait()
			close(w.done)
		}()
	})

	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *wizard) runSteps(
	ctx context.Context,
	req RunRequest,
	exec *models.ExecutionImpl,
) domainRuntime.ExecutionOutcome {
	for i, s := range req.Steps {
		if ctx.Err() != nil {
			return domainRuntime.ExecutionOutcomeCancelled
		}

		exec.Emit(Event{Kind: EventKindStepStarted, StepIndex: i})
		err := w.executeStep(ctx, req, s, exec)

		if err == nil {
			exec.Emit(Event{Kind: EventKindStepCompleted, StepIndex: i})
			continue
		}

		if ctx.Err() != nil {
			return domainRuntime.ExecutionOutcomeCancelled
		}

		exec.Emit(Event{Kind: EventKindStepFailed, StepIndex: i, Err: err})
		if s.ExitOnFailure() {
			return domainRuntime.ExecutionOutcomeFailed
		}
	}

	if ctx.Err() != nil {
		return domainRuntime.ExecutionOutcomeCancelled
	}

	return domainRuntime.ExecutionOutcomeSuccess
}

func (w *wizard) executeStep(
	ctx context.Context,
	req RunRequest,
	s domainstep.Step,
	exec *models.ExecutionImpl,
) error {
	stepReq := wizstep.Request{
		NSKey:   req.Namespace.String(),
		WorkDir: req.WorkDir,
		Vars:    req.Variables,
		OSArch:  domain.OS(goruntime.GOOS + "/" + goruntime.GOARCH),
		PID:     req.PID,
		Emit:    exec.Emit,
	}

	fn, ok := w.dispatch[s.Type()]
	if !ok {
		return ErrUnknownStepType
	}

	return fn(ctx, stepReq, s)
}

func adapt[S domainstep.Step](
	dispatch map[domainstep.StepType]DispatchFn,
	t domainstep.StepType,
	h wizstep.Handler[S],
) {
	dispatch[t] = func(ctx context.Context, req wizstep.Request, s domainstep.Step) error {
		typed, ok := s.(S)
		if !ok {
			return fmt.Errorf("adapt: step type mismatch: expected %T, got %T", *new(S), s)
		}
		return h.Execute(ctx, req, typed)
	}
}
