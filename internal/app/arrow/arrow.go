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
		opts UpdateOptions,
	) (UpdateResult, error)
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
	ns, arrow, constraint, err := svc.resolveForInstall(ctx, ns)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	return svc.catalog.Add(ctx, ns, arrow, true, constraint)
}

func (svc *arrowService) Update(
	ctx context.Context,
	ns domain.Namespace,
	opts UpdateOptions,
) (UpdateResult, error) {
	current, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("update: get current: %w", err)
	}
	if current == nil {
		return UpdateResult{}, fmt.Errorf("update: %w", apperrors.ErrNotFound)
	}

	oldArrow := &domain.Arrow{
		ArrowMeta: current.ArrowMeta,
		Variables: current.Variables,
		Netbridge: current.Netbridge,
		Targets:   current.Targets,
	}

	var latestRef string
	if current.InstalledConstraint != "" {
		latestRef, err = svc.manifold.ResolveConstraint(ctx, ns, current.InstalledConstraint)
		if err != nil {
			return UpdateResult{}, fmt.Errorf("update: resolve constraint: %w", err)
		}
	}

	if opts.UpgradeRef && latestRef != "" && latestRef != current.InstalledRef {
		return svc.upgradeVersion(ctx, ns, current, oldArrow, latestRef, opts)
	}

	return svc.updateManifest(ctx, ns, current, oldArrow, opts)
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

	type versionEntry struct {
		arrow domain.Arrow
		state domain.ArrowState
	}

	byBare := make(map[domain.Namespace][]versionEntry)
	for i := range arrows {
		a := &arrows[i]
		state := domain.ArrowStateAbsent
		rt, rtErr := svc.asynxRuntime.Get(ctx, a.Namespace.String())
		if rtErr == nil && rt.State != "" {
			state = rt.State
		} else if rtErr != nil && !errors.Is(rtErr, asynxModels.ErrNotFound) {
			return nil, rtErr
		}
		bare := a.Namespace.BareNamespace()
		byBare[bare] = append(byBare[bare], versionEntry{arrow: *a, state: state})
	}

	result := make([]ArrowListDTO, 0, len(byBare))
	for bare, versions := range byBare {
		hasUserInstalled := false
		for _, v := range versions {
			if v.arrow.UserInstalled {
				hasUserInstalled = true
				break
			}
		}
		if !hasUserInstalled {
			continue
		}

		vDTOs := make([]InstalledVersionDTO, 0, len(versions))
		for _, v := range versions {
			vDTOs = append(vDTOs, InstalledVersionDTO{
				Ref:         v.arrow.InstalledRef,
				Version:     v.arrow.Version,
				State:       v.state,
				InstalledAt: v.arrow.InstalledAt,
				Constraint:  v.arrow.InstalledConstraint,
			})
		}

		name := ""
		description := ""
		var tags []string
		if len(versions) > 0 {
			name = versions[0].arrow.Name
			description = versions[0].arrow.Description
			tags = versions[0].arrow.Tags
		}

		result = append(result, ArrowListDTO{
			Namespace:   bare,
			Name:        name,
			Description: description,
			Tags:        tags,
			Versions:    vDTOs,
		})
	}

	return result, nil
}

func (svc *arrowService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if ns.Ref() != "" {
		return svc.catalog.Get(ctx, ns)
	}

	arrows, err := svc.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	var latest *domain.Arrow
	for i := range arrows {
		a := &arrows[i]
		if a.Namespace.BareNamespace() != ns.BareNamespace() {
			continue
		}
		if latest == nil || a.InstalledAt.After(latest.InstalledAt) {
			latest = a
		}
	}
	if latest == nil {
		return nil, apperrors.ErrNotFound
	}
	return latest, nil
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

	runtime, runtimeErr := svc.asynxRuntime.Get(ctx, arrow.Namespace.String())
	if runtimeErr == nil && runtime.State != "" {
		state = runtime.State
	} else if runtimeErr != nil && !errors.Is(runtimeErr, asynxModels.ErrNotFound) {
		return nil, runtimeErr
	}

	if runtimeErr == nil {
		activeRun = runtime.Execution
		lastReturn = runtime.LastReturn
	}

	return &ArrowDetailDTO{
		Namespace:           arrow.Namespace,
		Name:                arrow.Name,
		Description:         arrow.Description,
		Tags:                arrow.Tags,
		Variables:           arrow.Variables,
		Targets:             arrow.Targets,
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
		resolvedNs, arrow, constraint, resolveErr := svc.resolveForInstall(ctx, entry.Namespace)
		if resolveErr != nil {
			return fmt.Errorf("install: resolve dep manifest %s: %w", entry.Namespace, resolveErr)
		}
		if addErr := svc.catalog.Add(
			ctx,
			resolvedNs,
			arrow,
			false,
			constraint,
		); addErr != nil && !errors.Is(addErr, apperrors.ErrAlreadyExists) {
			return fmt.Errorf("install: add dep to catalog %s: %w", entry.Namespace, addErr)
		}
	}

	if err := svc.deps.Execute(ctx, missing, ns); err != nil {
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
	return svc.execution.BeginExecution(ctx, ns, domain.Namespace(""), method, userVars)
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

func (svc *arrowService) resolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
) (
	resolvedNs domain.Namespace,
	arrow *domain.Arrow,
	constraint string,
	err error,
) {
	constraint = ""
	if ns.IsGlob() {
		constraint = ns.Ref()
		resolved, resolveErr := svc.manifold.ResolveConstraint(ctx, ns, ns.Ref())
		if resolveErr != nil {
			return ns, nil, "", fmt.Errorf("resolve constraint: %w", resolveErr)
		}
		ns = ns.WithRef(resolved)
	}
	arrow, err = svc.resolveManifest(ctx, ns)
	if err != nil {
		return ns, nil, "", fmt.Errorf("fetch manifest: %w", err)
	}
	return ns, arrow, constraint, nil
}

func (svc *arrowService) upgradeVersion(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	oldArrow *domain.Arrow,
	newRef string,
	opts UpdateOptions,
) (UpdateResult, error) {
	newRefNs := ns.BareNamespace().WithRef(newRef)

	newArrow, err := svc.resolveManifest(ctx, newRefNs)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("upgrade: fetch manifest: %w", err)
	}

	diff := svc.deps.DiffDeps(oldArrow, newArrow)

	addErr := svc.catalog.Add(ctx, newRefNs, newArrow, false, current.InstalledConstraint)
	if addErr != nil && !errors.Is(addErr, apperrors.ErrAlreadyExists) {
		return UpdateResult{}, fmt.Errorf("upgrade: add new version: %w", addErr)
	}

	if installErr := svc.execution.Install(ctx, newRefNs, nil); installErr != nil {
		return UpdateResult{}, fmt.Errorf("upgrade: install new version: %w", installErr)
	}

	safeToUninstall := svc.filterOrphans(ctx, edgesToNamespaces(diff.Removed), ns)

	result := UpdateResult{
		NewRef:              newRef,
		AddedDeps:           edgesToNamespaces(diff.Added),
		RemovedFromManifest: edgesToNamespaces(diff.Removed),
		SafeToUninstall:     safeToUninstall,
		ConstrainedDeps:     diff.Constrained,
	}

	if opts.InstallAdded {
		for _, dep := range diff.Added {
			_ = svc.Install(ctx, dep.Namespace, nil)
		}
	}
	if opts.UninstallOrphans {
		for _, dep := range safeToUninstall {
			_ = svc.Uninstall(ctx, dep, nil)
		}
		hasDeps, _ := svc.deps.HasDependents(ctx, ns, newRefNs)
		if !hasDeps {
			_ = svc.catalog.Remove(ctx, ns)
			_ = svc.execution.Uninstall(ctx, ns, nil)
		}
	}

	return result, nil
}

func (svc *arrowService) updateManifest(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	oldArrow *domain.Arrow,
	opts UpdateOptions,
) (UpdateResult, error) {
	newArrow, err := svc.resolveManifest(ctx, ns)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("update manifest: fetch: %w", err)
	}

	diff := svc.deps.DiffDeps(oldArrow, newArrow)
	safeToUninstall := svc.filterOrphans(ctx, edgesToNamespaces(diff.Removed), ns)

	if updateErr := svc.catalog.Update(ctx, ns, newArrow); updateErr != nil {
		return UpdateResult{}, fmt.Errorf("update manifest: catalog: %w", updateErr)
	}

	if opts.InstallAdded {
		for _, dep := range diff.Added {
			_ = svc.Install(ctx, dep.Namespace, nil)
		}
	}
	if opts.UninstallOrphans {
		for _, dep := range safeToUninstall {
			_ = svc.Uninstall(ctx, dep, nil)
		}
	}

	return UpdateResult{
		AddedDeps:           edgesToNamespaces(diff.Added),
		RemovedFromManifest: edgesToNamespaces(diff.Removed),
		SafeToUninstall:     safeToUninstall,
		ConstrainedDeps:     diff.Constrained,
	}, nil
}

func (svc *arrowService) filterOrphans(
	ctx context.Context,
	removed []domain.Namespace,
	excludeNs domain.Namespace,
) []domain.Namespace {
	var safe []domain.Namespace
	for _, ns := range removed {
		hasDeps, err := svc.deps.HasDependents(ctx, ns, excludeNs)
		if err != nil {
			slog.WarnContext(ctx, "filter orphans: check dependents failed",
				"dep", ns, "err", err)
			continue
		}
		if !hasDeps {
			safe = append(safe, ns)
		}
	}
	return safe
}

func edgesToNamespaces(
	edges []domain.DependencyEdge,
) []domain.Namespace {
	ns := make([]domain.Namespace, 0, len(edges))
	for _, e := range edges {
		ns = append(ns, e.Namespace)
	}
	return ns
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
