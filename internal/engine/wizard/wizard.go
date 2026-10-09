package wizard

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/models"
	wizrt "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	stepdeps "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/dependencies"
	stepdownload "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/download"
	stepexpose "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/expose"
	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
	stepportable "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable"
	steprun "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/run"
	stepsignal "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/signal"
)

// maxProbeDuration bounds every Probe call regardless of what its steps'
// manifest declares, or fails to declare. A probe runs synchronously on
// Add's own request goroutine (POST /v0/arrow/:ns), not on a supervised,
// cancellable-on-shutdown execution the way _install/_update do — so an
// undeclared or generous per-step timeout would otherwise block that
// endpoint for as long as the manifest's steps take, one manifest author's
// mistake away from indefinitely. A preinstalled check is meant to be a
// quick detect-or-not; this ceiling makes that true even if a future
// validator gap or a bypassed one lets an unbounded step through.
const maxProbeDuration = 30 * time.Second

// ErrNotLaunchable is what Launch returns for an arrow with no desktop entry to start.
var ErrNotLaunchable = shelf.ErrNotLaunchable

// Re-exports from internal/models — public API of the wizard package.
type (
	Event      = models.Event
	EventKind  = models.EventKind
	Execution  = models.Execution
	RunRequest = models.RunRequest
	PathStatus = shelf.PathStatus
)

const (
	EventKindStepStarted   = models.EventKindStepStarted
	EventKindStepCompleted = models.EventKindStepCompleted
	EventKindStepFailed    = models.EventKindStepFailed
	EventKindPID           = models.EventKindPID
	EventKindEnded         = models.EventKindEnded
	EventKindSurface       = models.EventKindSurface
	EventKindSurfaceClosed = models.EventKindSurfaceClosed
)

var (
	ErrUnknownStepType = models.ErrUnknownStepType
	ErrShuttingDown    = models.ErrShuttingDown
	ErrVacuousProbe    = models.ErrVacuousProbe

	// ErrChecksumMismatch is what a failed fetch step's event carries when the
	// downloaded content did not match the checksum its manifest declares.
	ErrChecksumMismatch = stepdownload.ErrChecksumMismatch
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

	PathStatus(
		ctx context.Context,
	) (PathStatus, error)
	SetupPath(
		ctx context.Context,
	) (PathStatus, error)

	// Launchable reports whether the arrow installed in workdir exposed a
	// desktop entry that Launch can start.
	Launchable(
		ctx context.Context,
		workdir string,
	) bool

	// Launch starts, detached from the daemon, the desktop entry the arrow
	// installed in workdir exposed. It fails with ErrNotLaunchable when there
	// is none.
	Launch(
		ctx context.Context,
		workdir string,
	) error
}

// DispatchFn is the untyped handler signature used in the dispatch table.
type DispatchFn = func(context.Context, wizstep.Request, domainstep.Step) error

type wizard struct {
	dispatch     map[domainstep.StepType]DispatchFn
	runtime      wizrt.Runtime
	shelf        shelf.Shelf
	exposer      stepexpose.Handler
	shutdownCtx  context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup // tracks one-shot executions and probes; see IsOneShotMethod
	shutdownOnce sync.Once
	done         chan struct{}
	mu           sync.Mutex
	shutting     bool
}

type options struct {
	shelf []shelf.Option
}

type Option func(*options)

// WithSandboxHome keeps every entry the wizard exposes, and the PATH it
// reports, inside home instead of the user's real home directory.
func WithSandboxHome(
	home string,
) Option {
	return func(o *options) { o.shelf = append(o.shelf, shelf.WithSandboxHome(home)) }
}

// depExec is the app-layer function that resolves DependenciesSteps;
// pass nil to treat dependency steps as no-ops.
func New(
	depExec stepdeps.Executor,
	extractMaxBytes int64,
	opts ...Option,
) (Wizard, error) {
	rt, err := wizrt.New()
	if err != nil {
		return nil, err
	}

	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}
	sh := shelf.New(cfg.shelf...)

	shutdownCtx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- cancel is stored in w.cancel and called in Shutdown

	w := &wizard{
		runtime:     rt,
		shelf:       sh,
		exposer:     stepexpose.NewHandler(sh),
		shutdownCtx: shutdownCtx,
		cancel:      cancel,
		dispatch:    make(map[domainstep.StepType]DispatchFn),
		done:        make(chan struct{}),
	}

	adapt(w.dispatch, domainstep.StepTypeRun, steprun.NewHandler(rt))
	adapt(w.dispatch, domainstep.StepTypeFetch, stepdownload.NewHandler())
	adapt(w.dispatch, domainstep.StepTypeSignal, stepsignal.NewHandler(rt))
	adapt(w.dispatch, domainstep.StepTypeDependencies, stepdeps.NewHandler(depExec))
	adapt(w.dispatch, domainstep.StepTypeExtract, stepextract.NewHandler(extractMaxBytes))
	adapt(w.dispatch, domainstep.StepTypePortable, stepportable.NewHandler(extractMaxBytes))
	adapt(w.dispatch, domainstep.StepTypeUnexpose, wizstep.Handler[domainstep.UnexposeStep](w.exposer))

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
	//
	// An execution that holds a ui surface is the exception: its interface is
	// reached only through this daemon's proxy and crash recovery cannot hand
	// the surface to the next daemon (RecordDetached drops the execution, and
	// with it the surface and the pid a stop would signal). Surviving would
	// leave an arrow app running that nothing can reach or stop, so it ends
	// with the daemon, like a one-shot method.
	oneShot := IsOneShotMethod(req.Method) || opensSurface(req.Steps)
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

// Probe runs req's steps as a yes/no question. A probe that has no workdir
// yet (a preinstalled check, before anything exists for its namespace) runs
// in a scratch directory of its own, never the daemon's working directory.
func (w *wizard) Probe(
	ctx context.Context,
	req RunRequest,
) error {
	if err := checkProbeAnswerable(req); err != nil {
		return err
	}

	if err := w.enterProbe(); err != nil {
		return err
	}
	defer w.wg.Done()

	if req.WorkDir == "" {
		scratch, err := os.MkdirTemp("", "quiver-probe-*")
		if err != nil {
			return fmt.Errorf("probe: scratch directory: %w", err)
		}
		defer os.RemoveAll(scratch) //nolint:errcheck // a leftover empty temp directory is harmless
		req.WorkDir = scratch
	}

	runCtx, cancel := context.WithTimeout(ctx, maxProbeDuration)
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

// checkProbeAnswerable refuses a probe that could only ever say yes, before it
// claims a shutdown slot or spawns anything. See ErrVacuousProbe: a probe's
// success is a decision to skip installing software, so a question nothing
// could answer must come back "not detected", never "already installed".
//
// Only run steps are examined. A fetch with no URL and a signal with no PID
// fail on their own; an empty command is the one shape that succeeds.
func checkProbeAnswerable(
	req RunRequest,
) error {
	if len(req.Steps) == 0 {
		return fmt.Errorf("probe: no steps: %w", ErrVacuousProbe)
	}

	osArch := currentOSArch().String()
	for i, s := range req.Steps {
		run, ok := s.(domainstep.RunStep)
		if !ok {
			continue
		}
		if run.Command.Resolve(osArch) == "" {
			return fmt.Errorf("probe step %d: empty command: %w", i, ErrVacuousProbe)
		}
	}

	return nil
}

// currentOSArch is the platform key every Overrideable in a step is resolved
// against.
func currentOSArch() domain.OS {
	return domain.OS(goruntime.GOOS + "/" + goruntime.GOARCH)
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

// opensSurface reports whether any of steps gives its run an interface.
func opensSurface(steps []domainstep.Step) bool {
	for _, s := range steps {
		if run, ok := s.(domainstep.RunStep); ok && run.UI != nil {
			return true
		}
	}
	return false
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

// Plan returns the steps a run of method executes for arrow on os: steps, plus
// the ones the wizard adds to expose the target's entries on install and update
// and to remove them first on uninstall.
func Plan(
	method string,
	arrow *domain.Arrow,
	os domain.OS,
	steps []domainstep.Step,
) []domainstep.Step {
	return stepexpose.Plan(method, arrow.Targets[os].Expose, arrow.Media.Icon, steps)
}

func (w *wizard) ProcessAlive(pid int) bool {
	return w.runtime.ProcessAlive(pid)
}

func (w *wizard) PathStatus(
	ctx context.Context,
) (PathStatus, error) {
	return w.shelf.PathStatus(ctx)
}

func (w *wizard) SetupPath(
	ctx context.Context,
) (PathStatus, error) {
	return w.shelf.SetupPath(ctx)
}

func (w *wizard) Launchable(
	ctx context.Context,
	workdir string,
) bool {
	return w.shelf.Launchable(ctx, workdir)
}

func (w *wizard) Launch(
	ctx context.Context,
	workdir string,
) error {
	return w.shelf.Launch(ctx, workdir)
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
	for i := 0; i < len(req.Steps); i++ {
		if ctx.Err() != nil {
			return domainRuntime.ExecutionOutcomeCancelled
		}

		if batch := leadingExposeSteps(req.Steps[i:]); len(batch) > 0 {
			w.expose(ctx, req, i, batch, exec)
			i += len(batch) - 1
			continue
		}

		s := req.Steps[i]
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
		slog.WarnContext(ctx, "wizard: non-fatal step failed", "ns", req.Namespace, "step", i, "type", s.Type(), "err", err)
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
	fn, ok := w.dispatch[s.Type()]
	if !ok {
		return ErrUnknownStepType
	}

	return fn(ctx, stepRequest(req, emit), s)
}

func stepRequest(
	req RunRequest,
	emit func(models.Event),
) wizstep.Request {
	return wizstep.Request{
		NSKey:   req.Namespace.String(),
		WorkDir: req.WorkDir,
		Vars:    req.Variables,
		OSArch:  currentOSArch(),
		PID:     req.PID,
		Emit:    emit,
	}
}

func leadingExposeSteps(
	steps []domainstep.Step,
) []domainstep.ExposeStep {
	var batch []domainstep.ExposeStep
	for _, s := range steps {
		e, ok := s.(domainstep.ExposeStep)
		if !ok {
			break
		}
		batch = append(batch, e)
	}
	return batch
}

func (w *wizard) expose(
	ctx context.Context,
	req RunRequest,
	first int,
	batch []domainstep.ExposeStep,
	exec *models.ExecutionImpl,
) {
	results := w.exposer.Expose(ctx, stepRequest(req, exec.Emit), batch)
	for k, res := range results {
		exec.Emit(Event{Kind: EventKindStepStarted, StepIndex: first + k})
		if res.Err != nil {
			exec.Emit(Event{Kind: EventKindStepFailed, StepIndex: first + k, Err: res.Err})
			slog.WarnContext(ctx, "wizard: expose step failed", "ns", req.Namespace, "step", first+k, "err", res.Err)
			continue
		}
		exec.Emit(Event{Kind: EventKindStepCompleted, StepIndex: first + k, Note: res.Note})
	}
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
