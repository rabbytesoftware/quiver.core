package arrow

import (
	"context"

	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func (svc *arrowService) runUninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) {
	arrow, err := svc.asynxArrow.Get(ctx, ns.String())
	if err != nil {
		return
	}

	vars, err := svc.resolveVariables(ctx, ns, &arrow.Manifest, "_uninstall", userVars)
	if err != nil {
		return
	}

	steps, availableIn := svc.stepsForMethod(arrow, "_uninstall")

	if err := svc.asynxRuntime.Send(ctx, arrowcmds.BeginExecution{
		Namespace:   ns,
		Method:      "_uninstall",
		AvailableIn: availableIn,
		Steps:       steps,
		Variables:   vars,
	}); err != nil {
		return
	}

	_, workDir, _ := svc.engines.Vault.GetArrow(ctx, ns)

	reporter := stepreporter.New(svc.asynxRuntime, ns)

	req := wizard.RunRequest{
		Namespace: ns,
		Variables: vars,
		Steps:     steps,
		WorkDir:   workDir,
	}

	execErr := svc.engines.Wizard.Execute(ctx, req, reporter)
	outcome := svc.mapOutcome(execErr)
	_ = svc.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
		Namespace: ns,
		Outcome:   outcome,
	})

	if outcome != domainRuntime.ExecutionOutcomeSuccess {
		return
	}

	entry, _, err := svc.engines.Vault.GetArrow(ctx, ns)
	if err != nil {
		_ = svc.engines.Vault.DeleteArrow(ctx, ns)
		return
	}

	allDeps := make([]domain.Namespace, 0, len(entry.Manifest.Dependencies)+len(entry.IndirectDependencies))
	allDeps = append(allDeps, entry.Manifest.Dependencies...)
	allDeps = append(allDeps, entry.IndirectDependencies...)

	orphaned := make(map[domain.Namespace]bool)
	for _, dep := range allDeps {
		hasDeps, err := svc.hasDependents(ctx, dep, ns)
		if err != nil || hasDeps {
			continue
		}
		orphaned[dep] = true
	}

	vaultResolver := func(resolveCtx context.Context, depNs domain.Namespace) ([]domain.Namespace, error) {
		depEntry, _, depErr := svc.engines.Vault.GetArrow(resolveCtx, depNs)
		if depErr != nil || depEntry == nil {
			return nil, nil
		}
		return depEntry.Manifest.Dependencies, nil
	}

	topoOrder, err := svc.engines.DepTree.Resolve(ctx, ns, deptree.ResolverFunc(vaultResolver))
	if err != nil {
		_ = svc.engines.Vault.DeleteArrow(ctx, ns)
		return
	}

	for i := len(topoOrder) - 1; i >= 0; i-- {
		dep := topoOrder[i]
		if dep == ns || !orphaned[dep] {
			continue
		}
		if uninstallErr := svc.executeSync(ctx, dep, "_uninstall", nil); uninstallErr != nil {
			continue
		}
		_ = svc.engines.Vault.DeleteArrow(ctx, dep)
	}

	_ = svc.engines.Vault.DeleteArrow(ctx, ns)
}

func (svc *arrowService) hasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	arrows, err := svc.catalog.List(ctx)
	if err != nil {
		return false, err
	}

	for _, arrow := range arrows {
		if arrow.Removed || arrow.Namespace == excludeNs || arrow.Namespace == ns {
			continue
		}

		rt, err := svc.asynxRuntime.Get(ctx, arrow.Namespace.String())
		if err != nil || rt.State == domain.ArrowStateAbsent || rt.Namespace == "" {
			continue
		}

		entry, _, err := svc.engines.Vault.GetArrow(ctx, arrow.Namespace)
		if err != nil {
			continue
		}

		for _, dep := range entry.Manifest.Dependencies {
			if dep == ns {
				return true, nil
			}
		}
		for _, dep := range entry.IndirectDependencies {
			if dep == ns {
				return true, nil
			}
		}
	}

	return false, nil
}
