package installer

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/runner"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// Installer owns Install, Uninstall, and CleanupAfterUninstall.
type Installer interface {
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
	CleanupAfterUninstall(
		ctx context.Context,
		ns domain.Namespace,
	)
}

type installerService struct {
	axArrow   asynx.Asynx[domain.Arrow]
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	vault     vault.Vault
	deptree   deptree.DepTree
	catalog   catalog.Catalog
	runner    runner.Runner
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
	dt deptree.DepTree,
	cat catalog.Catalog,
	r runner.Runner,
) (Installer, error) {
	return &installerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
		deptree:   dt,
		catalog:   cat,
		runner:    r,
	}, nil
}

func (inst *installerService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	arrow, err := inst.axArrow.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if errors.Is(err, asynxModels.ErrNotFound) || arrow.Namespace == "" {
		return fmt.Errorf("install: %w", apperrors.ErrNotFound)
	}

	rt, err := inst.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if rt.Ref != "" && rt.State != domain.ArrowStateAbsent {
		return fmt.Errorf("install: %w", apperrors.ErrStateViolation)
	}

	// Ensure the vault entry exists before execution begins so WORKDIR and
	// INSTALL_PATH are available to all steps. CleanupAfterUninstall removes
	// the vault entry, so a reinstall would otherwise have no working directory.
	// The vault manifest is sourced from the existing vault entry (fetched at add-time).
	vaultEntry, _, vaultErr := inst.vault.GetArrow(ctx, ns)
	if vaultErr != nil {
		return fmt.Errorf("install: vault entry missing: %w", vaultErr)
	}
	if _, err := inst.vault.PutArrow(ctx, ns, vaultEntry.Manifest); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	if err := inst.runner.BeginExecution(ctx, ns, "_install", userVars); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return nil
}

func (inst *installerService) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	rt, err := inst.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if errors.Is(err, asynxModels.ErrNotFound) || rt.Ref == "" || rt.State != domain.ArrowStateReady {
		return fmt.Errorf("uninstall: %w", apperrors.ErrStateViolation)
	}

	hasDeps, err := inst.catalog.HasDependents(ctx, ns, "")
	if err != nil {
		return err
	}
	if hasDeps {
		return fmt.Errorf("uninstall: %w", apperrors.ErrDependentsExist)
	}

	if err := inst.runner.BeginExecution(ctx, ns, "_uninstall", userVars); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	return nil
}

func (inst *installerService) CleanupAfterUninstall(
	ctx context.Context,
	ns domain.Namespace,
) {
	inst.cleanupAfterUninstall(ctx, ns)
}

func (inst *installerService) cleanupAfterUninstall(
	ctx context.Context,
	ns domain.Namespace,
) {
	entry, _, err := inst.vault.GetArrow(ctx, ns)
	if err != nil && !errors.Is(err, vault.ErrStale) {
		_ = inst.vault.DeleteArrow(ctx, ns)
		return
	}
	if entry == nil {
		_ = inst.vault.DeleteArrow(ctx, ns)
		return
	}

	directDeps := directDepsFromManifest(entry.Manifest)
	allDeps := directDeps

	orphaned := make(map[domain.Namespace]bool)
	for _, dep := range allDeps {
		hasDeps, depErr := inst.catalog.HasDependents(ctx, dep, ns)
		if depErr != nil || hasDeps {
			continue
		}
		orphaned[dep] = true
	}

	vaultResolver := func(resolveCtx context.Context, depNs domain.Namespace) ([]domain.Namespace, error) {
		depEntry, _, depErr := inst.vault.GetArrow(resolveCtx, depNs)
		if depErr != nil || depEntry == nil {
			return nil, nil
		}
		return directDepsFromManifest(depEntry.Manifest), nil
	}

	topoOrder, topoErr := inst.deptree.Resolve(ctx, ns, deptree.ResolverFunc(vaultResolver))
	if topoErr != nil {
		_ = inst.vault.DeleteArrow(ctx, ns)
		return
	}

	for i := len(topoOrder) - 1; i >= 0; i-- {
		dep := topoOrder[i]
		if dep == ns || !orphaned[dep] {
			continue
		}
		if uninstallErr := inst.runner.ExecuteSync(ctx, dep, "_uninstall", nil); uninstallErr != nil {
			continue
		}
		_ = inst.vault.DeleteArrow(ctx, dep)
	}

	_ = inst.vault.DeleteArrow(ctx, ns)
}

// directDepsFromManifest collects all unique dependency namespaces from all targets in a manifest.
func directDepsFromManifest(manifest *domain.ArrowManifest) []domain.Namespace {
	if manifest == nil {
		return nil
	}

	seen := make(map[domain.Namespace]bool)
	var deps []domain.Namespace

	for _, target := range manifest.Targets {
		for _, dep := range append(target.Tools, target.Services...) {
			bare := dep.Namespace.BareNamespace()
			if !seen[bare] {
				seen[bare] = true
				deps = append(deps, bare)
			}
		}
	}

	return deps
}
