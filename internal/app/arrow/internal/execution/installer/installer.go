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
	) error
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
	inst := &installerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
		deptree:   dt,
		catalog:   cat,
		runner:    r,
	}
	if err := inst.registerProjections(); err != nil {
		return nil, err
	}
	return inst, nil
}

func (inst *installerService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	arrow, err := inst.axArrow.Get(ctx, ns.String())
	if errors.Is(err, asynxModels.ErrNotFound) || arrow.Namespace == "" {
		return fmt.Errorf("install: %w", apperrors.ErrNotFound)
	}
	if err != nil {
		return err
	}

	rt, err := inst.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if rt.Namespace != "" && rt.State != domain.ArrowStateAbsent {
		return fmt.Errorf("install: %w", apperrors.ErrStateViolation)
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
	if errors.Is(err, asynxModels.ErrNotFound) || rt.Namespace == "" || rt.State != domain.ArrowStateReady {
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
) error {
	inst.cleanupAfterUninstall(ctx, ns)
	return nil
}

func (inst *installerService) cleanupAfterUninstall(
	ctx context.Context,
	ns domain.Namespace,
) {
	entry, _, err := inst.vault.GetArrow(ctx, ns)
	if err != nil {
		_ = inst.vault.DeleteArrow(ctx, ns)
		return
	}

	allDeps := make([]domain.Namespace, 0, len(entry.Manifest.Dependencies)+len(entry.IndirectDependencies))
	allDeps = append(allDeps, entry.Manifest.Dependencies...)
	allDeps = append(allDeps, entry.IndirectDependencies...)

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
		return depEntry.Manifest.Dependencies, nil
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
