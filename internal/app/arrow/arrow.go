package arrow

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
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
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	GetDetail(
		ctx context.Context,
		ns domain.Namespace,
	) (*ArrowDetailDTO, error)
	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)
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
	catalog      catalog.Catalog
	execution    execution.Execution
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	vault        vault.Vault
	hub          apphub.WebSocketHub
}

func (svc *arrowService) broadcastArrow(ctx context.Context, ns domain.Namespace) {
	if svc.hub == nil {
		return
	}
	arrow, err := svc.catalog.Get(ctx, ns)
	if err != nil || arrow == nil {
		return
	}
	svc.hub.BroadcastArrow(*arrow)
}

func (svc *arrowService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := svc.catalog.Add(ctx, ns); err != nil {
		return err
	}
	svc.broadcastArrow(ctx, ns)
	return nil
}

func (svc *arrowService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := svc.catalog.Update(ctx, ns); err != nil {
		return err
	}
	svc.broadcastArrow(ctx, ns)
	return nil
}

func (svc *arrowService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := svc.catalog.Remove(ctx, ns); err != nil {
		return err
	}
	svc.broadcastArrow(ctx, ns)
	return nil
}

func (svc *arrowService) List(
	ctx context.Context,
) ([]ArrowListDTO, error) {
	arrows, err := svc.catalog.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ArrowListDTO, 0, len(arrows))
	for _, arrow := range arrows {
		state := domain.ArrowStateAbsent
		runtime, runtimeErr := svc.asynxRuntime.Get(ctx, arrow.Namespace.String())
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

func (svc *arrowService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return svc.catalog.Get(ctx, ns)
}

func (svc *arrowService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*ArrowDetailDTO, error) {
	arrow, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get detail: %w", err)
	}
	if arrow == nil {
		return nil, fmt.Errorf("get detail: %w", apperrors.ErrNotFound)
	}

	state := domain.ArrowStateAbsent
	var activeRun *domainRuntime.RunRecord
	var lastReturn *domainRuntime.Return

	runtime, runtimeErr := svc.asynxRuntime.Get(ctx, ns.String())
	if runtimeErr == nil && runtime.State != "" {
		state = runtime.State
	} else if runtimeErr != nil && !errors.Is(runtimeErr, asynxModels.ErrNotFound) {
		return nil, runtimeErr
	}

	if runtimeErr == nil {
		activeRun = runtime.ActiveRun
		lastReturn = runtime.LastReturn
	}

	var indirectDeps []domain.Namespace
	if svc.vault != nil {
		entry, _, vaultErr := svc.vault.GetArrow(ctx, ns)
		if vaultErr == nil && entry != nil {
			indirectDeps = entry.IndirectDependencies
		}
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

func (svc *arrowService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return svc.catalog.HasDependents(ctx, ns, excludeNs)
}

func (svc *arrowService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	return svc.execution.Install(ctx, ns, userVars)
}

func (svc *arrowService) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	return svc.execution.Uninstall(ctx, ns, userVars)
}

func (svc *arrowService) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	return svc.execution.BeginExecution(ctx, ns, method, userVars)
}

func (svc *arrowService) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return svc.execution.Stop(ctx, ns)
}
