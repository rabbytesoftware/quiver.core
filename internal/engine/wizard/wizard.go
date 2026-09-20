package wizard

import (
	"context"
	"fmt"
	goruntime "runtime"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/models"
	wizrt "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	stepdeps "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/dependencies"
	stepdownload "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/download"
	steprun "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/run"
	stepsignal "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/signal"
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

var (
	ErrUnknownStepType = models.ErrUnknownStepType
	ErrShuttingDown    = models.ErrShuttingDown
)

type Wizard interface {
	// Start executes req.Steps in a background goroutine and returns immediately.
	// Events are delivered on the returned Execution; the goroutine exits when all
	// steps finish or ctx is cancelled.
	Start(
		ctx context.Context,
		req RunRequest,
	) Execution

	// Probe runs req.Steps on the calling goroutine and returns nil only when
	// every one of them succeeded. It is the ask-a-question counterpart to
	// Start: no goroutine, no Execution, no events, no PID reporting and no
	// supervision — the answer is the return value, and the first failing step
	// is the answer. req.Method is ignored; a probe is not a lifecycle method
	// and can never be invoked as one.
	//
	// It is shutdown-aware exactly as the one-shot lifecycle methods are: a
	// probe that arrives once Shutdown has begun is refused with
	// ErrShuttingDown, and one already running is cancelled and waited for.
	// That is the whole reason this is not simply a helper over Start —
	// IsOneShotMethod would classify a probe as a supervised process and leave
	// it running past the daemon's own shutdown.
	Probe(
		ctx context.Context,
		req RunRequest,
	) error

	// Shutdown cancels every active one-shot execution (_install, _uninstall,
	// _update, _stop) and waits for those goroutines to exit. It neither
	// cancels nor waits for an _execute or custom-method execution — those
	// are supervised processes meant to outlive the daemon's own shutdown.
	// If ctx expires before every one-shot goroutine finishes, Shutdown
	// returns ctx.Err() but cleanup continues in the background.
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
	wg           sync.WaitGroup // tracks one-shot executions and probes; see IsOneShotMethod
	shutdownOnce sync.Once
	done         chan struct{}
	mu           sync.Mutex
	shutting     bool
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

	w.mu.Lock()
	if w.shutting {
		w.mu.Unlock()
		exec.Finish(domainRuntime.ExecutionOutcomeCancelled)
		return exec
	}
	// A supervised process — the standard _execute method, or any custom
	// method a manifest declares under `methods:` — must outlive the
	// daemon's own shutdown: that is the entire premise the crash-recovery
	// path (RecoverTransients / RecordDetached) is built on, since it only
	// makes sense if such a process can already be alive-but-unmonitored
	// when the daemon comes back. Only the four one-shot lifecycle methods
	// are cancelled on shutdown; every other method name, known or custom,
	// survives by default. A surviving execution is counted in neither w.wg
	// nor w.shutdownCtx's cancellation, so Shutdown neither waits for it nor
	// asks it to exit.
	oneShot := IsOneShotMethod(req.Method)
	if oneShot {
		w.wg.Add(1)
	}
	go func() {
		if oneShot {
			defer w.wg.Done()
		}

		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		if oneShot {
			stop := context.AfterFunc(w.shutdownCtx, runCancel)
			defer stop()
		}

		outcome := w.runSteps(runCtx, req, exec)
		exec.Finish(outcome)
	}()
	w.mu.Unlock()

	return exec
}

func (w *wizard) Probe(
	ctx context.Context,
	req RunRequest,
) error {
	if err := w.enterProbe(); err != nil {
		return err
	}
	defer w.wg.Done()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(w.shutdownCtx, cancel)
	defer stop()

	for i, s := range req.Steps {
		if err := runCtx.Err(); err != nil {
			return fmt.Errorf("probe step %d: %w", i, err)
		}
		if err := w.executeStep(runCtx, req, s, nil); err != nil {
			return fmt.Errorf("probe step %d: %w", i, err)
		}
	}

	return nil
}

// enterProbe claims a slot in the shutdown wait group, or refuses when the
// wizard is already shutting down. The caller owns w.wg.Done on success.
func (w *wizard) enterProbe() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.shutting {
		return ErrShuttingDown
	}
	w.wg.Add(1)

	return nil
}

// IsOneShotMethod reports whether method is one of the four one-shot
// lifecycle methods the wizard cancels on shutdown and waits for in
// Shutdown. Every other method name — including _execute and any
// manifest-defined custom method — is a supervised process expected to
// survive the daemon's own shutdown. Exported so a test harness that must
// distinguish the same two cases (e.g. deciding which spawned processes it
// is responsible for cleaning up itself) does not need to duplicate this
// whitelist.
func IsOneShotMethod(method string) bool {
	switch method {
	case domain.MethodInstall, domain.MethodUninstall, domain.MethodUpdate, domain.MethodStop:
		return true
	default:
		return false
	}
}

func (w *wizard) ProcessAlive(pid int) bool {
	return w.runtime.ProcessAlive(pid)
}

func (w *wizard) Shutdown(
	ctx context.Context,
) error {
	w.shutdownOnce.Do(func() {
		w.mu.Lock()
		w.shutting = true
		w.cancel()
		w.mu.Unlock()
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
		err := w.executeStep(ctx, req, s, exec.Emit)

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

// executeStep dispatches one step. emit may be nil, which is how a probe runs:
// there is no Execution to carry mid-step events on, and handlers already treat
// a nil Emit as "nobody is listening".
func (w *wizard) executeStep(
	ctx context.Context,
	req RunRequest,
	s domainstep.Step,
	emit func(models.Event),
) error {
	stepReq := wizstep.Request{
		NSKey:   req.Namespace.String(),
		WorkDir: req.WorkDir,
		Vars:    req.Variables,
		OSArch:  domain.OS(goruntime.GOOS + "/" + goruntime.GOARCH),
		PID:     req.PID,
		Emit:    emit,
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
