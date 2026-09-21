package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler"
	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/core/shutdown"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

type Runtime interface {
	BeginInstall(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	BeginStop(
		ctx context.Context,
		ns domain.Namespace,
	) error
	BeginUninstall(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	BeginUpdate(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error

	RuntimeExists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)

	Start(
		ctx context.Context,
	)
	// Shutdown stops the wizard, waits for the one-shot-method drain goroutines
	// to finish (see waitDrains), then drains the runtime aggregate. It does not
	// wait for the drain goroutine of an _execute or custom-method execution —
	// those are supervised processes meant to outlive the daemon's own shutdown,
	// so waiting for them here would defeat that. Every phase runs even when an
	// earlier one fails, and each gets its own share of ctx rather than all
	// three sharing it: a process that refuses to stop makes the wizard spend a
	// whole shared budget, and the aggregate would then drain on a dead context
	// — returning at once, leaving its remaining writes to land on an
	// already-closing store.
	Shutdown(
		ctx context.Context,
	) error

	OnRuntimeEnded(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeBegun(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeRecovered(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeDetached(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimePIDRecorded(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeOutdated(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeOutdatedCleared(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeStepAdvanced(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	GetState(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.ArrowState, error)
	GetRuntime(
		ctx context.Context,
		ns domain.Namespace,
	) (*domainRuntime.ArrowRuntime, error)
	ListenEnded(
		ctx context.Context,
		ns domain.Namespace,
	) (<-chan domainRuntime.ArrowRuntime, func(), error)
	MarkOutdated(
		ctx context.Context,
		ns domain.Namespace,
		addedDeps []domain.Namespace,
		removedDeps []domain.Namespace,
	) error
	// MarkReady lands ns's runtime aggregate at Ready without an install ever
	// having run, the same outcome MarkPreinstalled records for a preinstalled
	// detection. Its caller is the arrow.upgraded reaction for a swap raised
	// after this arrow's own update lifecycle already finished successfully,
	// see domain.Arrow.AlreadyReady. lastReturn, when non-nil, carries that
	// completed update's outcome onto the new aggregate; pass nil when there
	// is none to carry.
	MarkReady(
		ctx context.Context,
		ns domain.Namespace,
		lastReturn *domainRuntime.Return,
	) error
	Forget(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

type runtimeRepository struct {
	axRuntime             asynx.Asynx[domainRuntime.ArrowRuntime]
	wizard                wizardPkg.Wizard
	assembler             assembler.Assembler
	hasDependents         HasDependentsFn
	listArrows            ListArrowsFn
	listRuntimeAggregates ListRuntimeAggregatesFn
	drainWg               sync.WaitGroup // tracks only one-shot-method drains; see waitDrains
	drainMu               sync.Mutex
	drainClosed           bool
}

func New(
	getArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	v vault.Vault,
	markInstalled MarkInstalledFn,
	markUninstalled MarkUninstalledFn,
	markLastUsed MarkLastUsedFn,
	hasDependents HasDependentsFn,
	listArrows ListArrowsFn,
	os domain.OS,
	listRuntimeAggregates ListRuntimeAggregatesFn,
) (Runtime, error) {
	repo := &runtimeRepository{
		axRuntime:             axRuntime,
		wizard:                w,
		assembler:             assembler.New(assembler.GetArrowFn(getArrow), axRuntime, v, nil, os),
		hasDependents:         hasDependents,
		listArrows:            listArrows,
		listRuntimeAggregates: listRuntimeAggregates,
	}

	hooks := runtimeinternal.CatalogHooks{
		MarkInstalled:         markInstalled,
		MarkUninstalled:       markUninstalled,
		MarkLastUsed:          markLastUsed,
		ReconcileVersionBadge: ReconcileVersionBadge(getArrow, axRuntime),
	}

	if err := runtimeinternal.RegisterReactions(
		axRuntime, hooks, w, repo.tryAddDrain,
	); err != nil {
		return nil, fmt.Errorf("runtime: register reactions: %w", err)
	}

	return repo, nil
}

// MarkPreinstalled returns a function that lands ns's runtime aggregate at
// Ready without an install, for an arrow whose preinstalled lifecycle found it
// already present on this machine. It is idempotent: a namespace already Ready
// stays Ready, with whatever return history it had.
//
// It is a function over the aggregate rather than a method on Runtime because
// the arrow repository needs it before runtime.New can be called at all —
// runtime.New itself takes the arrow repository's MarkInstalled and friends, so
// the two cannot each be constructed first. Handing out a closure over
// axRuntime breaks that cycle in the same shape repositories/container.go's
// arrowGetter already breaks it in the other direction.
//
// The send is a SendWait. Its one caller is arrowService.Add, running on the
// caller's own goroutine rather than inside an asynx worker, so waiting here
// blocks nobody: the cross-instance circular wait documented on
// internal/app/container.go's newAsynx needs an arrow worker blocked on a
// runtime send, which this deliberately is not. Waiting is also the point —
// Add must not write the catalog row until the runtime already reads Ready.
func MarkPreinstalled(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) func(ctx context.Context, ns domain.Namespace) error {
	return func(ctx context.Context, ns domain.Namespace) error {
		_, err := axRuntime.SendWait(ctx, runtimecmds.RecordPreinstalled{Namespace: ns})
		if err == nil {
			return nil
		}
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
			return fmt.Errorf("mark preinstalled %s: %w", ns, apperrors.ErrStateViolation)
		}

		return fmt.Errorf("mark preinstalled %s: %w", ns, err)
	}
}

// ForgetPreinstalled returns a function that clears ns's runtime aggregate
// entirely, for the one moment Add's preinstalled path needs it: a probe that
// finds nothing for a namespace that is not yet catalogued. Without this, a
// prior Add that reached MarkPreinstalled but died before its catalog row was
// ever written (a crash, a store error) leaves a Ready runtime behind that no
// later Add — including one whose own probe finds nothing — ever revisits,
// because nothing in this codebase reconciles a runtime against a catalog row
// that was never written. A negative probe on an uncatalogued namespace is the
// one place that orphan can still be observed, so it is the one place that
// clears it.
//
// It is built the same way MarkPreinstalled is, as a closure over axRuntime
// rather than a method on Runtime, for the same construction-order reason:
// the arrow repository needs this before runtime.New can be called at all.
// The existence check mirrors Runtime.Forget's own — Asynx's Forget on an
// aggregate that was never written is not something this path needs to ask
// for, and the common case (no prior attempt ever detected anything) hits
// exactly that branch.
func ForgetPreinstalled(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) func(ctx context.Context, ns domain.Namespace) error {
	return func(ctx context.Context, ns domain.Namespace) error {
		exists, err := axRuntime.Exists(ctx, ns.String())
		if err != nil {
			return fmt.Errorf("forget preinstalled %s: %w", ns, err)
		}
		if !exists {
			return nil
		}
		if err := axRuntime.Forget(ctx, ns.String()); err != nil {
			return fmt.Errorf("forget preinstalled %s: %w", ns, err)
		}
		return nil
	}
}

// stateViolation builds an ErrStateViolation naming the operation and the
// arrow's current state, so the user sees why a transition was rejected.
func (s *runtimeRepository) stateViolation(ctx context.Context, op string, ns domain.Namespace) error {
	state, _ := s.GetState(ctx, ns)
	return apperrors.NewStateViolation(op, string(state))
}

// methodOp turns a runtime method name into the verb shown to the user.
func methodOp(method string) string {
	op := strings.TrimPrefix(method, "_")
	if op == "execute" {
		return "run"
	}
	return op
}

func (s *runtimeRepository) BeginInstall(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) error {
	resolved, err := s.assembler.Assemble(ctx, ns, domain.MethodInstall, vars)
	if err != nil {
		return fmt.Errorf("begin install: %w", err)
	}
	_, err = s.axRuntime.Send(ctx, runtimecmds.BeginInstall{
		Namespace:   ns,
		ExecutionID: uuid.NewString(),
		Steps:       resolved.Steps,
		Variables:   resolved.Variables,
		WorkDir:     resolved.WorkDir,
	})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
			return s.stateViolation(ctx, "install", ns)
		}
		return fmt.Errorf("begin install: %w", err)
	}
	return nil
}

func (s *runtimeRepository) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error {
	resolved, err := s.assembler.Assemble(ctx, ns, method, vars)
	if err != nil {
		return fmt.Errorf("begin execution: %w", err)
	}
	_, err = s.axRuntime.Send(ctx, runtimecmds.BeginExecution{
		Namespace:   ns,
		ExecutionID: uuid.NewString(),
		Method:      method,
		Steps:       resolved.Steps,
		Variables:   resolved.Variables,
		AvailableIn: resolved.AvailableIn,
		WorkDir:     resolved.WorkDir,
	})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
			return s.stateViolation(ctx, methodOp(method), ns)
		}
		return fmt.Errorf("begin execution: %w", err)
	}
	return nil
}

func (s *runtimeRepository) BeginStop(ctx context.Context, ns domain.Namespace) error {
	resolved, err := s.assembler.Assemble(ctx, ns, domain.MethodStop, nil)
	if err != nil {
		if !errors.Is(err, apperrors.ErrMethodNotFound) {
			return fmt.Errorf("begin stop: %w", err)
		}
		resolved = assembler.ResolvedExecution{}
	}
	cmd := runtimecmds.BeginStop{
		Namespace:   ns,
		ExecutionID: uuid.NewString(),
		Steps:       resolved.Steps,
		Variables:   resolved.Variables,
		WorkDir:     resolved.WorkDir,
	}
	// Retry on ErrPipelineFailed: drainExecution goroutine may concurrently send
	// AdvanceStep/RecordPID events, causing an OCC conflict. ErrValidation is never
	// retried — it means the arrow is not in a stoppable state.
	for range 5 {
		_, err = s.axRuntime.Send(ctx, cmd)
		if err == nil {
			return nil
		}
		if errors.Is(err, asynxModels.ErrValidation) {
			return s.stateViolation(ctx, "stop", ns)
		}
		if !errors.Is(err, asynxModels.ErrPipelineFailed) {
			return err
		}
	}
	return apperrors.ErrStateViolation
}

func (s *runtimeRepository) BeginUninstall(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) error {
	resolved, err := s.assembler.Assemble(ctx, ns, domain.MethodUninstall, vars)
	if err != nil {
		return fmt.Errorf("begin uninstall: %w", err)
	}
	_, err = s.axRuntime.Send(ctx, runtimecmds.BeginUninstall{
		Namespace:   ns,
		ExecutionID: uuid.NewString(),
		Steps:       resolved.Steps,
		Variables:   resolved.Variables,
		WorkDir:     resolved.WorkDir,
	})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
			return s.stateViolation(ctx, "uninstall", ns)
		}
		return fmt.Errorf("begin uninstall: %w", err)
	}
	return nil
}

func (s *runtimeRepository) BeginUpdate(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) error {
	resolved, err := s.assembler.Assemble(ctx, ns, domain.MethodUpdate, vars)
	if err != nil {
		return fmt.Errorf("begin update: %w", err)
	}
	_, err = s.axRuntime.Send(ctx, runtimecmds.BeginUpdate{
		Namespace:   ns,
		ExecutionID: uuid.NewString(),
		Steps:       resolved.Steps,
		Variables:   resolved.Variables,
		WorkDir:     resolved.WorkDir,
	})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
			return s.stateViolation(ctx, "update", ns)
		}
		return fmt.Errorf("begin update: %w", err)
	}
	return nil
}

func (s *runtimeRepository) RuntimeExists(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	_, err := s.axRuntime.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *runtimeRepository) Start(ctx context.Context) {
	runtimeinternal.RecoverTransients(ctx, s.listArrows, s.listRuntimeAggregates, s.axRuntime, s.wizard)
}

// tryAddDrain registers one drain goroutine, if Shutdown should wait for it.
// Returns (nil, false) if Shutdown has already closed the gate — the caller
// must not start the goroutine. Otherwise it returns (done, true): done must
// be called when the goroutine exits, but is only wired into s.drainWg for a
// one-shot method (_install, _uninstall, _update, _stop). A drain for
// _execute or a custom method — a supervised process that survives the
// daemon's own shutdown, per wizard.Shutdown's own contract — gets a no-op
// done instead, so waitDrains never waits on a goroutine that only returns
// once that survivor's Events() channel closes, which during shutdown it
// correctly never does.
func (s *runtimeRepository) tryAddDrain(method string) (func(), bool) {
	s.drainMu.Lock()
	defer s.drainMu.Unlock()
	if s.drainClosed {
		return nil, false
	}
	if !wizardPkg.IsOneShotMethod(method) {
		return func() {}, true
	}
	s.drainWg.Add(1)
	return s.drainWg.Done, true
}

func (s *runtimeRepository) Shutdown(ctx context.Context) error {
	return shutdown.Split(ctx, "runtime shutdown", []shutdown.Phase{
		{Name: "wizard", Run: s.shutdownWizard},
		{Name: "drain", Run: s.waitDrains},
		{Name: "aggregate", Run: s.axRuntime.Shutdown},
	})
}

func (s *runtimeRepository) shutdownWizard(ctx context.Context) error {
	if s.wizard == nil {
		return nil
	}
	return s.wizard.Shutdown(ctx)
}

// waitDrains closes the drain gate, then waits only for the one-shot-method
// drain goroutines already past it (_install, _uninstall, _update, _stop) —
// bounded by ctx, which carries this phase's own share of the shutdown budget
// rather than whatever the wizard left behind. It does not wait for the drain
// goroutine of an _execute or custom-method execution: that goroutine only
// returns once its Execution's Events() channel closes, which happens when
// wizard.Shutdown lets that execution finish — and per wizard.Shutdown's own
// contract, a supervised process like this is deliberately left running, not
// finished, so waiting for it here would just re-introduce the same bug one
// layer up.
//
// The bound on the drains it does wait for is still not optional. A one-shot
// execution's drain goroutine is only guaranteed to finish once wizard.Shutdown
// itself returns, and wizard.Shutdown can report a timeout for that same
// execution — leaving its drainExecution partner still ranging and still
// counted here. An unbounded Wait would therefore hang the whole shutdown
// sequence in exactly the case the caller gave us a deadline for.
func (s *runtimeRepository) waitDrains(ctx context.Context) error {
	s.drainMu.Lock()
	s.drainClosed = true
	s.drainMu.Unlock()

	done := make(chan struct{})
	go func() {
		s.drainWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *runtimeRepository) OnRuntimeEnded(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.ended.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) OnRuntimeBegun(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.begun.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) OnRuntimeRecovered(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.recovered.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) OnRuntimeDetached(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.detached.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) OnRuntimePIDRecorded(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.pid_recorded.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) OnRuntimeOutdated(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.outdated.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) OnRuntimeOutdatedCleared(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.outdated_cleared.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) OnRuntimeStepAdvanced(fn func(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
),
) error {
	_, err := s.axRuntime.Subscribe(asynx.Topic("runtime.step_advanced.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *runtimeRepository) GetState(
	ctx context.Context,
	ns domain.Namespace,
) (domain.ArrowState, error) {
	got, err := s.axRuntime.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return domain.ArrowStateAbsent, nil
		}
		return "", err
	}
	return got.State, nil
}

func (s *runtimeRepository) GetRuntime(
	ctx context.Context,
	ns domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
	got, err := s.axRuntime.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &got, nil
}

func (s *runtimeRepository) ListenEnded(
	ctx context.Context,
	ns domain.Namespace,
) (<-chan domainRuntime.ArrowRuntime, func(), error) {
	ch, unsub, err := s.axRuntime.Listen("runtime.ended."+ns.String(), 1)
	if err != nil {
		return nil, nil, err
	}
	resultCh := make(chan domainRuntime.ArrowRuntime, 1)
	go func() {
		if evt, ok := <-ch; ok {
			resultCh <- evt.Aggregate
		}
	}()
	return resultCh, unsub, nil
}

func (s *runtimeRepository) MarkOutdated(
	ctx context.Context,
	ns domain.Namespace,
	addedDeps []domain.Namespace,
	removedDeps []domain.Namespace,
) error {
	_, err := s.axRuntime.Send(ctx, runtimecmds.MarkOutdated{
		Namespace:   ns,
		AddedDeps:   addedDeps,
		RemovedDeps: removedDeps,
	})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
			return apperrors.ErrStateViolation
		}
		return err
	}
	return nil
}

// MarkReady sends the same RecordPreinstalled command MarkPreinstalled sends,
// as a plain method rather than a construction-time closure: its caller
// (usecases/runtime.go onArrowUpgraded) already holds a Runtime built by
// New, so none of MarkPreinstalled's construction-order constraint applies
// here.
func (s *runtimeRepository) MarkReady(ctx context.Context, ns domain.Namespace, lastReturn *domainRuntime.Return) error {
	_, err := s.axRuntime.SendWait(ctx, runtimecmds.RecordPreinstalled{Namespace: ns, LastReturn: lastReturn})
	if err == nil {
		return nil
	}
	if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
		return fmt.Errorf("mark ready %s: %w", ns, apperrors.ErrStateViolation)
	}
	return fmt.Errorf("mark ready %s: %w", ns, err)
}

func (s *runtimeRepository) Forget(ctx context.Context, ns domain.Namespace) error {
	exists, err := s.axRuntime.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("forget runtime: %w", err)
	}
	if !exists {
		return nil
	}
	return s.axRuntime.Forget(ctx, ns.String())
}
