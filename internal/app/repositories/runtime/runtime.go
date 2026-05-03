package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	runtimeinternal "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/assembler"
	runtimecmds "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

type Runtime interface {
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	Uninstall(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
	Stop(
		ctx context.Context,
		ns domain.Namespace,
	) error
	RuntimeExists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)

	Start(
		ctx context.Context,
	)
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
	ClearOutdated(
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
}

func New(
	getArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	v vault.Vault,
	markInstalled MarkInstalledFn,
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

	if err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, w, &repo.drainWg); err != nil {
		return nil, fmt.Errorf("runtime: register reactions: %w", err)
	}

	return repo, nil
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
		Method:      method,
		Steps:       resolved.Steps,
		Variables:   resolved.Variables,
		AvailableIn: resolved.AvailableIn,
		WorkDir:     resolved.WorkDir,
	})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) ||
			errors.Is(err, asynxModels.ErrPipelineFailed) {
			return fmt.Errorf("begin execution: %w", apperrors.ErrStateViolation)
		}
		return fmt.Errorf("begin execution: %w", err)
	}
	return nil
}

func (s *runtimeRepository) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	hasDeps, err := s.hasDependents(ctx, ns)
	if err != nil {
		return err
	}
	if hasDeps {
		return fmt.Errorf("uninstall: %w", apperrors.ErrDependentsExist)
	}
	return s.BeginExecution(ctx, ns, domain.MethodUninstall, userVars)
}

func (s *runtimeRepository) Stop(ctx context.Context, ns domain.Namespace) error {
	resolved, err := s.assembler.Assemble(ctx, ns, domain.MethodStop, nil)
	if err != nil {
		if !errors.Is(err, apperrors.ErrMethodNotFound) {
			return fmt.Errorf("stop: %w", err)
		}
		resolved = assembler.ResolvedExecution{}
	}
	_, err = s.axRuntime.Send(ctx, runtimecmds.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodStop,
		AvailableIn: []domain.ArrowState{domain.ArrowStateRunning},
		Steps:       resolved.Steps,
		Variables:   resolved.Variables,
		WorkDir:     resolved.WorkDir,
	})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) ||
			errors.Is(err, asynxModels.ErrPipelineFailed) {
			return apperrors.ErrStateViolation
		}
		return err
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

func (s *runtimeRepository) Shutdown(ctx context.Context) error {
	if s.wizard != nil {
		if err := s.wizard.Shutdown(ctx); err != nil {
			return fmt.Errorf("runtime shutdown: wizard: %w", err)
		}
	}
	s.drainWg.Wait()
	return s.axRuntime.Shutdown(ctx)
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

func (s *runtimeRepository) ClearOutdated(
	ctx context.Context,
	ns domain.Namespace,
) error {
	_, err := s.axRuntime.Send(ctx, runtimecmds.ClearOutdated{Namespace: ns})
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
			return apperrors.ErrStateViolation
		}
		return err
	}
	return nil
}
