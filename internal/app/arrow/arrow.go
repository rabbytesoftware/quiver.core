package arrow

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

// ArrowService manages arrow lifecycle: registration, manifest updates, installation, and execution.
type ArrowService interface {
	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Update(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	List(
		ctx context.Context,
	) ([]ArrowListDTO, error)
	GetDetail(
		ctx context.Context,
		ns domain.Namespace,
	) (*ArrowDetailDTO, error)
	Install(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
	Uninstall(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		userVars map[string]string,
	) error
	Stop(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

type arrowService struct {
	asynxArrow   asynx.Asynx[domain.Arrow]
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]

	catalog arrowstore.ArrowCatalog
	engines engine.Container
	os      domain.OS
}

func (svc *arrowService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add arrow: %w", ErrInvalidNamespace)
	}

	manifest, _, err := svc.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("add arrow: %w", ErrFetchFailed)
	}

	if err := svc.asynxArrow.Send(ctx, arrowcmds.AddArrow{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		return fmt.Errorf("add arrow: %w", err)
	}

	return nil
}

func (svc *arrowService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	current, err := svc.asynxArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("update arrow: %w", ErrNotFound)
		}
		return fmt.Errorf("update arrow: %w", err)
	}

	if current.Removed {
		return fmt.Errorf("update arrow: %w", ErrAlreadyRemoved)
	}

	runtime, err := svc.asynxRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return fmt.Errorf("update arrow: %w", err)
	}

	if runtime.Namespace != "" && runtime.State != domain.ArrowStateReady {
		return fmt.Errorf("update arrow: %w", ErrStateViolation)
	}

	manifest, err := svc.engines.Manifold.ResolveArrow(ctx, ns)
	if err != nil {
		return fmt.Errorf("update arrow: %w", ErrFetchFailed)
	}

	if _, err := svc.engines.Vault.PutArrow(ctx, ns, manifest, nil); err != nil {
		return fmt.Errorf("update arrow: %w", err)
	}

	if err := svc.asynxArrow.Send(ctx, arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		return fmt.Errorf("update arrow: %w", err)
	}

	return nil
}

func (svc *arrowService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	current, err := svc.asynxArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("remove arrow: %w", ErrNotFound)
		}
		return fmt.Errorf("remove arrow: %w", err)
	}

	if current.Removed {
		return fmt.Errorf("remove arrow: %w", ErrAlreadyRemoved)
	}

	runtime, err := svc.asynxRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return fmt.Errorf("remove arrow: %w", err)
	}

	if runtime.Namespace != "" {
		state := runtime.State
		if state != domain.ArrowStateAbsent && state != domain.ArrowStateRemoved && state != "" {
			return fmt.Errorf("remove arrow: %w", ErrStateViolation)
		}
	}

	if err := svc.asynxArrow.Send(ctx, arrowcmds.RemoveArrow{Namespace: ns}); err != nil {
		return fmt.Errorf("remove arrow: %w", err)
	}

	_ = svc.engines.Vault.DeleteArrow(ctx, ns) // best-effort: removal proceeds even if vault cleanup fails

	return nil
}

func (s *arrowService) List(
	ctx context.Context,
) ([]ArrowListDTO, error) {
	arrows, err := s.catalog.List()
	if err != nil {
		return nil, err
	}

	result := make([]ArrowListDTO, 0, len(arrows))
	for _, arrow := range arrows {
		if arrow.Removed {
			continue
		}

		state := domain.ArrowStateAbsent
		runtime, runtimeErr := s.asynxRuntime.Get(ctx, arrow.Namespace.String())
		if runtimeErr == nil && runtime.State != "" {
			state = runtime.State
		} else if runtimeErr != nil && !errors.Is(runtimeErr, asynxModels.ErrNotFound) {
			return nil, runtimeErr
		}

		result = append(result, ArrowListDTO{
			Namespace:   arrow.Namespace,
			Name:        arrow.Manifest.Name,
			Version:     arrow.Manifest.Version,
			Description: arrow.Manifest.Description,
			State:       state,
			Tags:        arrow.Manifest.Tags,
			Removed:     arrow.Removed,
		})
	}

	return result, nil
}

func (s *arrowService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*ArrowDetailDTO, error) {
	arrow, err := s.catalog.Get(ns)
	if err != nil {
		return nil, err
	}
	if arrow == nil {
		return nil, fmt.Errorf("get detail: %w", ErrNotFound)
	}

	state := domain.ArrowStateAbsent
	var activeRun *domainRuntime.RunRecord
	var lastReturn *domainRuntime.Return

	runtime, runtimeErr := s.asynxRuntime.Get(ctx, ns.String())
	if runtimeErr == nil && runtime.State != "" {
		state = runtime.State
	} else if runtimeErr != nil && !errors.Is(runtimeErr, asynxModels.ErrNotFound) {
		return nil, runtimeErr
	}

	if !errors.Is(runtimeErr, asynxModels.ErrNotFound) && runtimeErr == nil {
		activeRun = runtime.ActiveRun
		lastReturn = runtime.LastReturn
	}

	var indirectDeps []domain.Namespace
	entry, _, vaultErr := s.engines.Vault.GetArrow(ctx, ns)
	if vaultErr == nil && entry != nil {
		indirectDeps = entry.IndirectDependencies
	}

	return &ArrowDetailDTO{
		Namespace:            arrow.Namespace,
		Manifest:             arrow.Manifest,
		State:                state,
		Removed:              arrow.Removed,
		ActiveRun:            activeRun,
		LastReturn:           lastReturn,
		IndirectDependencies: indirectDeps,
	}, nil
}

func (svc *arrowService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	arrow, err := svc.asynxArrow.Get(ctx, ns.String())
	if errors.Is(err, asynxModels.ErrNotFound) || arrow.Namespace == "" {
		return fmt.Errorf("install: %w", ErrNotFound)
	}
	if err != nil {
		return err
	}

	rt, err := svc.asynxRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if rt.Namespace != "" && rt.State != domain.ArrowStateAbsent {
		return fmt.Errorf("install: %w", ErrStateViolation)
	}

	go svc.runInstall(ctx, ns, userVars)
	return nil
}

func (svc *arrowService) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	rt, err := svc.asynxRuntime.Get(ctx, ns.String())
	if errors.Is(err, asynxModels.ErrNotFound) || rt.Namespace == "" || rt.State != domain.ArrowStateReady {
		return fmt.Errorf("uninstall: %w", ErrStateViolation)
	}

	hasDeps, err := svc.hasDependents(ctx, ns, "")
	if err != nil {
		return err
	}
	if hasDeps {
		return fmt.Errorf("uninstall: %w", ErrDependentsExist)
	}

	go svc.runUninstall(ctx, ns, userVars)
	return nil
}

func (svc *arrowService) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	req, reporter, err := svc.beginExecution(ctx, ns, method, userVars)
	if err != nil {
		return err
	}

	go func() {
		execErr := svc.engines.Wizard.Execute(context.Background(), *req, reporter)
		outcome := svc.mapOutcome(execErr)
		_ = svc.asynxRuntime.Send(context.Background(), arrowcmds.EndExecution{Namespace: ns, Outcome: outcome})
		if execErr != nil {
			svc.handleExecutionError(context.Background(), ns, method, execErr)
		}
	}()

	return nil
}

func (svc *arrowService) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	runtime, err := svc.asynxRuntime.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("stop: %w", ErrNotFound)
		}
		return fmt.Errorf("stop: %w", err)
	}

	if runtime.State != domain.ArrowStateRunning {
		return fmt.Errorf("stop: %w", ErrStateViolation)
	}

	if err := svc.asynxRuntime.Send(ctx, arrowcmds.MarkStopping{Namespace: ns}); err != nil {
		return fmt.Errorf("stop: %w", err)
	}

	return nil
}
