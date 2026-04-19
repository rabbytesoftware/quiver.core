package arrow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	appDeps "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
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
	catalog         catalog.Catalog
	execution       execution.Execution
	deps            appDeps.Deps
	resolveManifest manifest.ResolveFunc
	asynxRuntime    asynx.Asynx[domainRuntime.ArrowRuntime]
	vault           vault.Vault
	manifold        manifold.Manifold
}

func (svc *arrowService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	m, err := svc.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("add: resolve manifest: %w", err)
	}
	return svc.catalog.Add(ctx, ns, m, true, "")
}

func (svc *arrowService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	existing, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	oldManifest := existing

	newManifest, err := svc.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("update: resolve manifest: %w", err)
	}

	diff := svc.deps.DiffDeps(oldManifest, newManifest)

	for _, added := range diff.Added {
		if err := svc.Install(ctx, added.Namespace, nil); err != nil {
			return fmt.Errorf("update: install added dep %s: %w", added.Namespace, err)
		}
	}

	for _, removed := range diff.Removed {
		hasDeps, depErr := svc.deps.HasDependents(ctx, removed.Namespace, ns)
		if depErr != nil {
			slog.WarnContext(ctx, "update: check dependents failed", "dep", removed.Namespace, "err", depErr)
			continue
		}
		if hasDeps {
			continue
		}
		if err := svc.Uninstall(ctx, removed.Namespace, nil); err != nil {
			return fmt.Errorf("update: uninstall removed dep %s: %w", removed.Namespace, err)
		}
	}

	return svc.catalog.Update(ctx, ns, newManifest)
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
	for i := range arrows {
		arrow := &arrows[i]
		state := domain.ArrowStateAbsent
		runtime, runtimeErr := svc.asynxRuntime.Get(ctx, arrow.Namespace.String())
		if runtimeErr == nil && runtime.State != "" {
			state = runtime.State
		} else if runtimeErr != nil && !errors.Is(runtimeErr, asynxModels.ErrNotFound) {
			return nil, runtimeErr
		}

		meta := arrow.ArrowMeta

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

	var m domain.Arrow
	if svc.vault != nil {
		entry, _, vaultErr := svc.vault.GetArrow(ctx, ns)
		if vaultErr == nil && entry != nil && entry.Manifest != nil {
			m = *entry.Manifest
		}
	}

	return &ArrowDetailDTO{
		Namespace:           arrow.Namespace,
		Name:                m.Name,
		Description:         m.Description,
		Tags:                m.Tags,
		Variables:           m.Variables,
		Targets:             m.Targets,
		InstalledAt:         arrow.InstalledAt,
		InstalledRef:        arrow.InstalledRef,
		InstalledConstraint: arrow.InstalledConstraint,
		UserInstalled:       arrow.UserInstalled,
		State:               state,
		ActiveRun:           activeRun,
		LastReturn:          lastReturn,
	}, nil
}

func (svc *arrowService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return svc.deps.HasDependents(ctx, ns, excludeNs)
}

func (svc *arrowService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	plan, err := svc.deps.Resolve(ctx, ns)
	if err != nil {
		return fmt.Errorf("install: resolve deps: %w", err)
	}

	var missing appDeps.Plan
	for _, entry := range plan {
		if !svc.catalog.IsInstalled(ctx, entry.Namespace) {
			missing = append(missing, entry)
		}
	}

	for _, entry := range missing {
		m, mErr := svc.resolveManifest(ctx, entry.Namespace)
		if mErr != nil {
			return fmt.Errorf("install: resolve dep manifest %s: %w", entry.Namespace, mErr)
		}
		if err := svc.catalog.Add(ctx, entry.Namespace, m, false, ""); err != nil && !errors.Is(err, apperrors.ErrAlreadyExists) {
			return fmt.Errorf("install: add dep to catalog %s: %w", entry.Namespace, err)
		}
	}

	if err := svc.deps.Execute(ctx, missing); err != nil {
		return fmt.Errorf("install: execute deps: %w", err)
	}

	return svc.execution.Install(ctx, ns, userVars)
}

func (svc *arrowService) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	hasDeps, err := svc.deps.HasDependents(ctx, ns, domain.Namespace(""))
	if err != nil {
		return err
	}
	if hasDeps {
		return fmt.Errorf("uninstall: %w", apperrors.ErrDependentsExist)
	}
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

	m, err := svc.manifold.ParseArrow(data)
	if err != nil {
		return fmt.Errorf("seed arrow: %w: %w", apperrors.ErrInvalidManifest, err)
	}

	err = svc.catalog.Add(ctx, ns, m, true, "")
	if err == nil {
		return nil
	}
	if !errors.Is(err, apperrors.ErrAlreadyExists) {
		return fmt.Errorf("seed arrow: %w", err)
	}
	// If it already exists, update it instead.
	return svc.catalog.Update(ctx, ns, m)
}

func (svc *arrowService) ValidateManifest(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) (*ValidationResult, error) {
	m, err := svc.manifold.ParseArrow(data)
	if err == nil {
		supported := make([]domain.OS, 0, len(m.Targets))
		for os := range m.Targets {
			supported = append(supported, os)
		}
		unsupported := make([]domain.OS, 0)
		for _, os := range domain.AllOS() {
			if _, ok := m.Targets[os]; !ok {
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

func (svc *arrowService) cleanupAfterUninstall(
	ctx context.Context,
	ns domain.Namespace,
) {
	orphans, err := svc.deps.Orphans(ctx, ns)
	if err != nil {
		return
	}
	for _, orphan := range orphans {
		_ = svc.Uninstall(ctx, orphan, nil)
	}
}
