package arrow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
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
		userInstalled *bool,
	) ([]ArrowListDTO, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	GetDetail(
		ctx context.Context,
		ns domain.Namespace,
	) (*ArrowDetailDTO, error)
	GetManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*ArrowManifestDTO, error)
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
	Shutdown(ctx context.Context) error
}

type arrowService struct {
	catalog         catalog.Catalog
	execution       execution.Execution
	deps            appDeps.Deps
	resolveManifest manifest.ResolveFunc
	asynxArrow      asynx.Asynx[domain.Arrow]
	asynxRuntime    asynx.Asynx[domainRuntime.ArrowRuntime]
	vault           vault.Vault
	manifold        manifold.Manifold
	closeDB         func() // closes the underlying SQL connection on Shutdown
}

func (svc *arrowService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	ns, arrow, constraint, err := svc.resolveForInstall(ctx, ns)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	arrow.UserInstalled = true
	arrow.InstalledConstraint = constraint
	return svc.addArrowCommand(ctx, ns, arrow, constraint)
}

func (svc *arrowService) addArrowCommand(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	constraint string,
) error {
	existing, getErr := svc.asynxArrow.Get(ctx, ns.String())
	if getErr == nil {
		if !existing.UserInstalled {
			_, sendErr := svc.asynxArrow.Send(
				ctx,
				arrowcmds.SetUserInstalled{Namespace: ns},
			)
			return sendErr
		}

		return nil
	}

	cmd := arrowcmds.AddArrow{
		Namespace:           ns,
		ArrowMeta:           arrow.ArrowMeta,
		Variables:           arrow.Variables,
		Netbridge:           arrow.Netbridge,
		Targets:             arrow.Targets,
		DirectInstall:       arrow.UserInstalled,
		InstalledConstraint: constraint,
	}
	if _, sendErr := svc.asynxArrow.Send(ctx, cmd); sendErr != nil {
		if errors.Is(sendErr, asynxModels.ErrValidation) ||
			errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
			return fmt.Errorf("add arrow: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add arrow: %w", sendErr)
	}
	return nil
}

func (svc *arrowService) Update(
	ctx context.Context,
	ns domain.Namespace,
	opts UpdateOptions,
) (UpdateResult, error) {
	vm, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("update: get current: %w", err)
	}
	if vm == nil {
		return UpdateResult{}, fmt.Errorf("update: %w", apperrors.ErrNotFound)
	}

	current := &vm.Metadata
	oldArrow := &domain.Arrow{
		ArrowMeta: current.ArrowMeta,
		Variables: current.Variables,
		Netbridge: current.Netbridge,
		Targets:   current.Targets,
	}

	var latestRef string
	if current.InstalledConstraint != "" {
		latestRef, err = svc.manifold.ResolveConstraint(
			ctx,
			ns,
			current.InstalledConstraint,
		)
		if err != nil {
			return UpdateResult{},
				fmt.Errorf("update: resolve constraint: %w", err)
		}
	}

	if opts.UpgradeRef && latestRef != "" &&
		latestRef != current.InstalledRef {
		return svc.upgradeVersion(
			ctx,
			ns,
			current,
			oldArrow,
			latestRef,
			opts,
		)
	}

	return svc.updateManifest(
		ctx,
		ns,
		current,
		oldArrow,
		opts,
	)
}

func (svc *arrowService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := svc.asynxArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	if !exists {
		return fmt.Errorf("remove: %w", apperrors.ErrNotFound)
	}

	// Check if anything depends on this arrow
	hasDeps, err := svc.deps.HasDependents(ctx, ns, "")
	if err != nil {
		return fmt.Errorf("remove: check dependents: %w", err)
	}
	if hasDeps {
		return fmt.Errorf("remove: %w", apperrors.ErrDependentsExist)
	}

	// Check if arrow is currently running/installing
	rt, rtErr := svc.asynxRuntime.Get(ctx, ns.String())
	if rtErr == nil && rt.State != "" && rt.State != domain.ArrowStateAbsent && rt.State != domain.ArrowStateRemoved {
		return fmt.Errorf("remove: %w", apperrors.ErrStateViolation)
	}

	return svc.asynxArrow.Forget(ctx, ns.String())
}

func (svc *arrowService) List(
	ctx context.Context,
	userInstalled *bool,
) ([]ArrowListDTO, error) {
	vms, err := svc.catalog.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ArrowListDTO, 0, len(vms))
	for _, vm := range vms {
		// Filter by userInstalled flag
		// nil → user-installed only (default). &true → user-installed. &false → deps only.
		hasUserInstalled := false
		for _, versionRef := range vm.Versions {
			if versionRef.Metadata.UserInstalled {
				hasUserInstalled = true
				break
			}
		}

		if userInstalled == nil || *userInstalled {
			if !hasUserInstalled {
				continue
			}
		} else {
			if hasUserInstalled {
				continue
			}
		}

		// Enrich versions with runtime state
		vDTOs := make([]InstalledVersionDTO, 0, len(vm.Versions))
		for _, versionRef := range vm.Versions {
			state := domain.ArrowStateAbsent
			rt, rtErr := svc.asynxRuntime.Get(ctx, versionRef.Namespace.String())
			if rtErr == nil && rt.State != "" {
				state = rt.State
			} else if rtErr != nil && !errors.Is(rtErr, asynxModels.ErrNotFound) {
				return nil, rtErr
			}

			vDTOs = append(vDTOs, InstalledVersionDTO{
				Ref:         versionRef.Metadata.InstalledRef,
				Version:     versionRef.Metadata.Version,
				State:       state,
				InstalledAt: versionRef.Metadata.InstalledAt,
				Constraint:  versionRef.Metadata.InstalledConstraint,
			})
		}

		result = append(result, ArrowListDTO{
			Namespace:   vm.Namespace,
			Name:        vm.Metadata.Name,
			Description: vm.Metadata.Description,
			Tags:        vm.Metadata.Tags,
			Versions:    vDTOs,
		})
	}

	return result, nil
}

func (svc *arrowService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	vm, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	if vm == nil {
		return nil, apperrors.ErrNotFound
	}
	return &vm.Metadata, nil
}

func (svc *arrowService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*ArrowDetailDTO, error) {
	err := svc.asynxRuntime.Preload(ctx, ns.String())
	if err != nil {
		return nil, err
	}

	vm, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get detail: %w", err)
	}
	if vm == nil {
		return nil, fmt.Errorf("get detail: %w", apperrors.ErrNotFound)
	}

	var metadataArrow *domain.Arrow
	runtimeNs := vm.Metadata.Namespace

	if ns.Ref() != "" {
		found := false
		for _, versionRef := range vm.Versions {
			if versionRef.Namespace.String() == ns.String() {
				metadataArrow = &versionRef.Metadata
				runtimeNs = versionRef.Namespace
				found = true
				break
			}
		}
		if !found { // Requested version not in catalog
			return nil, fmt.Errorf("get detail: %w", apperrors.ErrNotFound)
		}
	} else {
		metadataArrow = &vm.Metadata
	}

	state := domain.ArrowStateAbsent
	var activeRun *domainRuntime.Execution
	var lastReturn *domainRuntime.Return

	runtime, runtimeErr := svc.asynxRuntime.Get(ctx, runtimeNs.String())
	if runtimeErr == nil &&
		runtime.State != "" {
		state = runtime.State
	} else if runtimeErr != nil &&
		!errors.Is(runtimeErr, asynxModels.ErrNotFound) {
		return nil, runtimeErr
	}

	if runtimeErr == nil {
		activeRun = runtime.Execution
		lastReturn = runtime.LastReturn
	}

	return &ArrowDetailDTO{
		Namespace:           metadataArrow.Namespace,
		Name:                metadataArrow.Name,
		Description:         metadataArrow.Description,
		Tags:                metadataArrow.Tags,
		Variables:           metadataArrow.Variables,
		Targets:             metadataArrow.Targets,
		InstalledAt:         metadataArrow.InstalledAt,
		InstalledRef:        metadataArrow.InstalledRef,
		InstalledConstraint: metadataArrow.InstalledConstraint,
		UserInstalled:       metadataArrow.UserInstalled,
		State:               state,
		ActiveRun:           activeRun,
		LastReturn:          lastReturn,
	}, nil
}

func (svc *arrowService) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*ArrowManifestDTO, error) {
	if ns.Ref() != "" {
		return nil, fmt.Errorf("get manifest: %w", apperrors.ErrInvalidNamespace)
	}

	arrow, err := svc.resolveManifest(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	return &ArrowManifestDTO{
		Namespace:   arrow.Namespace,
		Name:        arrow.Name,
		Description: arrow.Description,
		Version:     arrow.Version,
		Tags:        arrow.Tags,
		Variables:   arrow.Variables,
		Targets:     arrow.Targets,
		Manifest:    arrow,
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
	exists, err := svc.asynxArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if !exists {
		return fmt.Errorf("install: %w", apperrors.ErrNotFound)
	}

	plan, err := svc.deps.Resolve(ctx, ns)
	if err != nil {
		return fmt.Errorf("install: resolve deps: %w", err)
	}

	var missing appDeps.Plan
	for _, entry := range plan {
		_, rtErr := svc.asynxRuntime.Get(ctx, entry.Namespace.String())
		if rtErr != nil && errors.Is(rtErr, asynxModels.ErrNotFound) {
			missing = append(missing, entry)
		}
	}

	for _, entry := range missing {
		resolvedNs, arrow, constraint, resolveErr := svc.resolveForInstall(
			ctx,
			entry.Namespace,
		)
		if resolveErr != nil {
			return fmt.Errorf("install: resolve dep manifest %s: %w",
				entry.Namespace,
				resolveErr,
			)
		}

		arrow.UserInstalled = false
		if sendErr := svc.addArrowCommand(
			ctx,
			resolvedNs,
			arrow,
			constraint,
		); sendErr != nil && !errors.Is(sendErr, apperrors.ErrAlreadyExists) {
			return fmt.Errorf("install: add dep to catalog %s: %w",
				entry.Namespace,
				sendErr,
			)
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
	hasDeps, err := svc.deps.HasDependents(
		ctx,
		ns,
		domain.Namespace(""),
	)
	if err != nil {
		return err
	}

	if hasDeps {
		return fmt.Errorf("uninstall: %w", apperrors.ErrDependentsExist)
	}

	return svc.execution.Uninstall(
		ctx,
		ns,
		userVars,
	)
}

func (svc *arrowService) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	return svc.execution.BeginExecution(
		ctx,
		ns,
		domain.Namespace(""),
		method,
		userVars,
	)
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

	if ns.Ref() == "" && m.Version != "" {
		ns = ns.WithRef(m.Version)
	}

	if _, err := svc.vault.PutArrow(ctx, ns, m); err != nil {
		return fmt.Errorf("seed arrow: vault write: %w", err)
	}

	m.UserInstalled = true
	err = svc.addArrowCommand(ctx, ns, m, "")
	if err == nil {
		return nil
	}

	if !errors.Is(err, apperrors.ErrAlreadyExists) {
		return fmt.Errorf("seed arrow: %w", err)
	}

	// If it already exists, send UpdateArrowManifest command instead.
	cmd := arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: m.ArrowMeta,
		Variables: m.Variables,
		Netbridge: m.Netbridge,
		Targets:   m.Targets,
	}
	_, err = svc.asynxArrow.Send(ctx, cmd)
	return err
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

func (svc *arrowService) Shutdown(ctx context.Context) error {
	if err := svc.asynxArrow.Shutdown(ctx); err != nil {
		return fmt.Errorf("arrow shutdown: axArrow: %w", err)
	}
	if err := svc.asynxRuntime.Shutdown(ctx); err != nil {
		return fmt.Errorf("arrow shutdown: axRuntime: %w", err)
	}
	if svc.closeDB != nil {
		svc.closeDB()
	}
	return nil
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

	// Transfer the installation state from the old version to the new one.
	// If v2 is already installed (both versions coexist), skip the rename —
	// v2's installation state should not be overwritten.
	// If v2 was only pre-cached by resolveManifest (just arrow.json, no install),
	// remove that cache entry so RenameArrow can move v1's slot into place.
	_, rtErr := svc.asynxRuntime.Get(ctx, newRefNs.String())
	if rtErr != nil && errors.Is(rtErr, asynxModels.ErrNotFound) {
		_ = svc.vault.DeleteArrow(ctx, newRefNs)
		if err := svc.vault.RenameArrow(ctx, ns, newRefNs); err != nil {
			return UpdateResult{}, fmt.Errorf("upgrade: rename vault entry: %w", err)
		}
	}

	if _, err := svc.vault.PutArrow(ctx, newRefNs, newArrow); err != nil {
		return UpdateResult{}, fmt.Errorf("upgrade: write new manifest: %w", err)
	}

	newArrow.UserInstalled = false
	sendErr := svc.addArrowCommand(ctx, newRefNs, newArrow, current.InstalledConstraint)
	if sendErr != nil && !errors.Is(sendErr, apperrors.ErrAlreadyExists) {
		return UpdateResult{}, fmt.Errorf("upgrade: add new version: %w", sendErr)
	}

	hasUpdate := false
	for _, target := range newArrow.Targets {
		if len(target.Lifecycle.Update) > 0 {
			hasUpdate = true
			break
		}
	}

	if hasUpdate {
		if err := svc.execution.BeginExecution(
			ctx,
			newRefNs,
			domain.Namespace(""),
			domain.MethodUpdate,
			nil,
		); err != nil {
			return UpdateResult{}, fmt.Errorf("upgrade: begin _update: %w", err)
		}
	} else {
		if err := svc.Install(ctx, newRefNs, nil); err != nil {
			return UpdateResult{}, fmt.Errorf("upgrade: install new version: %w", err)
		}
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
			_ = svc.Add(ctx, dep.Namespace)
			_ = svc.Install(ctx, dep.Namespace, nil)
		}
	}

	// Retire the old version: vault entry was already renamed to newRefNs,
	// so ns has no files. Remove it from the asynx unconditionally.
	// Must happen before orphan uninstall so HasDependents sees no edge from ns.
	if err := svc.asynxArrow.Forget(ctx, ns.String()); err != nil {
		slog.WarnContext(ctx, "upgrade: retire old version failed", "ns", ns, "err", err)
	}

	// Always uninstall deps that the new version no longer needs.
	// Runs after Retire so ns no longer appears as a dependent in the dep store.
	for _, dep := range safeToUninstall {
		_ = svc.Uninstall(ctx, dep, nil)
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

	cmd := arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: newArrow.ArrowMeta,
		Variables: newArrow.Variables,
		Netbridge: newArrow.Netbridge,
		Targets:   newArrow.Targets,
	}
	if _, sendErr := svc.asynxArrow.Send(ctx, cmd); sendErr != nil {
		return UpdateResult{}, fmt.Errorf("update manifest: send: %w", sendErr)
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
		// Use execution.Uninstall directly (not svc.Uninstall) because:
		// - Orphans() already verified these have no other dependents besides ns
		// - The parent's dep-edge row may still be present here, which would cause
		//   svc.Uninstall's HasDependents check to falsely block the cascade.
		_ = svc.execution.Uninstall(ctx, orphan, nil)
	}
}
