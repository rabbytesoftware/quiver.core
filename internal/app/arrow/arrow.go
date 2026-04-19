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
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
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
	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error
	ValidateManifest(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) (*ValidationResult, error)
}

type arrowService struct {
	catalog      catalog.Catalog
	execution    execution.Execution
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	vault        vault.Vault
	manifold     manifold.Manifold
}

func (svc *arrowService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return svc.catalog.Add(ctx, ns)
}

func (svc *arrowService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return svc.catalog.Update(ctx, ns)
}

func (svc *arrowService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return svc.catalog.Remove(ctx, ns)
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

		var meta domain.ArrowMeta
		if v, ok := (&arrow).VersionFor(arrow.Namespace.Ref()); ok {
			meta = v.ArrowMeta
		}

		result = append(result, ArrowListDTO{
			Namespace:   arrow.Namespace,
			Name:        meta.Name,
			Version:     meta.Version,
			Description: meta.Description,
			State:       state,
			Tags:        meta.Tags,
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
	var activeRun *domainRuntime.Execution
	var lastReturn *domainRuntime.Return

	runtime, runtimeErr := svc.asynxRuntime.Get(ctx, ns.String())
	if runtimeErr == nil && runtime.State != "" {
		state = runtime.State
	} else if runtimeErr != nil && !errors.Is(runtimeErr, asynxModels.ErrNotFound) {
		return nil, runtimeErr
	}

	if runtimeErr == nil {
		activeRun = runtime.Execution
		lastReturn = runtime.LastReturn
	}

	var manifest domain.ArrowManifest
	if svc.vault != nil {
		entry, _, vaultErr := svc.vault.GetArrow(ctx, ns)
		if vaultErr == nil && entry != nil && entry.Manifest != nil {
			manifest = *entry.Manifest
		}
	}

	return &ArrowDetailDTO{
		Namespace:  arrow.Namespace,
		Manifest:   manifest,
		State:      state,
		ActiveRun:  activeRun,
		LastReturn: lastReturn,
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

func (svc *arrowService) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("seed arrow: %w", apperrors.ErrInvalidNamespace)
	}

	manifest, err := svc.manifold.ParseArrow(data)
	if err != nil {
		return fmt.Errorf("seed arrow: %w", apperrors.ErrInvalidManifest)
	}

	err = svc.catalog.AddWithManifest(ctx, ns, manifest)
	if errors.Is(err, apperrors.ErrAlreadyExists) {
		return svc.catalog.UpdateWithManifest(ctx, ns, manifest)
	}
	return err
}

func (svc *arrowService) ValidateManifest(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) (*ValidationResult, error) {
	manifest, err := svc.manifold.ParseArrow(data)
	if err == nil {
		supported := make([]domain.OS, 0, len(manifest.Targets))
		for os := range manifest.Targets {
			supported = append(supported, os)
		}
		unsupported := make([]domain.OS, 0)
		for _, os := range domain.AllOS() {
			if _, ok := manifest.Targets[os]; !ok {
				unsupported = append(unsupported, os)
			}
		}
		return &ValidationResult{
			Valid:                true,
			SupportedPlatforms:   supported,
			UnsupportedPlatforms: unsupported,
		}, nil
	}

	var asmErrs ruleset.RuleErrors
	if errors.As(err, &asmErrs) {
		errs := make([]ValidationError, len(asmErrs))
		for i, ae := range asmErrs {
			errs[i] = ValidationError{
				Field:   ae.Field,
				Rule:    ae.Rule,
				Message: ae.Message,
			}
		}
		return &ValidationResult{
			Valid:                false,
			Errors:               errs,
			SupportedPlatforms:   []domain.OS{},
			UnsupportedPlatforms: []domain.OS{},
		}, nil
	}

	return &ValidationResult{
		Valid: false,
		Errors: []ValidationError{{
			Rule:    "parse_error",
			Message: err.Error(),
		}},
		SupportedPlatforms:   []domain.OS{},
		UnsupportedPlatforms: []domain.OS{},
	}, nil
}
