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
	// Shutdown stops the wizard, waits for every drain goroutine to finish, then
	// drains the runtime aggregate. Every phase runs even when an earlier one
	// fails, and each gets its own share of ctx rather than all three sharing it:
	// a process that refuses to stop makes the wizard spend a whole shared budget,
	// and the aggregate would then drain on a dead context — returning at once,
	// leaving its remaining writes to land on an already-closing store.
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
	Forget(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

type runtimeRepository struct {
	axRuntime     asynx.Asynx[domainRuntime.ArrowRuntime]
	wizard        wizardPkg.Wizard
	assembler     assembler.Assembler
	hasDependents HasDependentsFn
	listArrows    ListArrowsFn
	drainWg       sync.WaitGroup
	drainMu       sync.Mutex
	drainClosed   bool
}

func New(
	getArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	v vault.Vault,
	markInstalled MarkInstalledFn,
	markUninstalled MarkUninstalledFn,
	hasDependents HasDependentsFn,
	listArrows ListArrowsFn,
	os domain.OS,
) (Runtime, error) {
	repo := &runtimeRepository{
		axRuntime:     axRuntime,
		wizard:        w,
		assembler:     assembler.New(assembler.GetArrowFn(getArrow), axRuntime, v, nil, os),
		hasDependents: hasDependents,
		listArrows:    listArrows,
	}

	if err := runtimeinternal.RegisterReactions(
		axRuntime, markInstalled, markUninstalled, w, repo.tryAddDrain,
	); err != nil {
		return nil, fmt.Errorf("runtime: register reactions: %w", err)
	}

	return repo, nil
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
		Variables:   vars,
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
	runtimeinternal.RecoverTransients(ctx, s.listArrows, s.axRuntime, s.wizard)
}

// tryAddDrain registers one drain goroutine with the WaitGroup.
// Returns (Done, true) if registration succeeded, or (nil, false) if Shutdown
// has already closed the gate — the caller must not start the goroutine.
func (s *runtimeRepository) tryAddDrain() (func(), bool) {
	s.drainMu.Lock()
	defer s.drainMu.Unlock()
	if s.drainClosed {
		return nil, false
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

// waitDrains closes the drain gate, then waits for the goroutines already past
// it — bounded by ctx, which carries this phase's own share of the shutdown
// budget rather than whatever the wizard left behind.
//
// The bound is not optional. wizard.Shutdown reports a timeout precisely when an
// execution goroutine is still running, and that goroutine is the one that
// closes its Execution's events channel, so its drainExecution partner is still
// ranging and still counted here. An unbounded Wait would therefore hang the
// whole shutdown sequence in exactly the case the caller gave us a deadline for.
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
