package arrow

import (
	"context"
	"errors"

	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func (svc *arrowService) runInstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) {
	arrow, err := svc.asynxArrow.Get(ctx, ns.String())
	if err != nil {
		return
	}

	vars, err := svc.resolveVariables(ctx, ns, &arrow.Manifest, "_install", userVars)
	if err != nil {
		return
	}

	steps, _ := svc.stepsForMethod(arrow, "_install")

	if err := svc.asynxRuntime.Send(ctx, arrowcmds.BeginExecution{
		Namespace:   ns,
		Method:      "_install",
		AvailableIn: nil,
		Steps:       steps,
		Variables:   vars,
	}); err != nil {
		return
	}

	_ = svc.asynxRuntime.Send(ctx, arrowcmds.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusRunning,
	})

	resolver := svc.buildDepResolver()
	orderedDeps, err := svc.deptree(ctx, ns, resolver)
	if err != nil {
		errStr := err.Error()
		_ = svc.asynxRuntime.Send(ctx, arrowcmds.AdvanceStep{
			Namespace: ns,
			StepIndex: 0,
			ToStatus:  domainRuntime.StepStatusFailed,
			Error:     &errStr,
		})
		_ = svc.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
			Namespace: ns,
			Outcome:   domainRuntime.ExecutionOutcomeFailed,
		})
		return
	}

	_ = svc.asynxRuntime.Send(ctx, arrowcmds.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusCompleted,
	})

	var installed []domain.Namespace

	for _, dep := range orderedDeps {
		if dep == ns {
			continue
		}

		rt, err := svc.asynxRuntime.Get(ctx, dep.String())
		if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
			// Treat unexpected errors as "not installed" — proceed with install attempt
			err = nil
		}
		if err == nil && rt.Namespace != "" && rt.State != domain.ArrowStateAbsent {
			continue
		}

		manifest, _, err := svc.resolveManifest(ctx, dep)
		if err != nil {
			svc.rollbackInstalled(ctx, installed)
			_ = svc.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
				Namespace: ns,
				Outcome:   domainRuntime.ExecutionOutcomeFailed,
			})
			return
		}

		existing, getErr := svc.asynxArrow.Get(ctx, dep.String())
		if errors.Is(getErr, asynxModels.ErrNotFound) || existing.Namespace == "" {
			_ = svc.asynxArrow.Send(ctx, arrowcmds.AddArrow{
				Namespace: dep,
				Manifest:  *manifest,
			})
		}

		if err := svc.executeSync(ctx, dep, "_install", nil); err != nil {
			svc.rollbackInstalled(ctx, installed)
			_ = svc.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
				Namespace: ns,
				Outcome:   domainRuntime.ExecutionOutcomeFailed,
			})
			return
		}

		installed = append(installed, dep)
	}

	_, workDir, _ := svc.vault.GetArrow(ctx, ns)

	installSteps := arrow.Manifest.Lifecycle.Install
	reporter := stepreporter.New(svc.asynxRuntime, ns, 1)

	req := wizard.RunRequest{
		Namespace: ns,
		Variables: vars,
		Steps:     installSteps,
		WorkDir:   workDir,
	}

	execErr := svc.wizard.Execute(ctx, req, reporter)
	outcome := svc.mapOutcome(execErr)
	_ = svc.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
		Namespace: ns,
		Outcome:   outcome,
	})

	if outcome == domainRuntime.ExecutionOutcomeSuccess {
		svc.updateIndirectDeps(ctx, ns, arrow.Manifest, orderedDeps)
	}
}

func (svc *arrowService) rollbackInstalled(ctx context.Context, installed []domain.Namespace) {
	for i := len(installed) - 1; i >= 0; i-- {
		dep := installed[i]
		rt, err := svc.asynxRuntime.Get(ctx, dep.String())
		if err != nil || rt.State != domain.ArrowStateReady {
			continue
		}
		_ = svc.executeSync(ctx, dep, "_uninstall", nil)
	}
}

func (svc *arrowService) updateIndirectDeps(
	ctx context.Context,
	ns domain.Namespace,
	manifest domain.ArrowManifest,
	deptreeResult []domain.Namespace,
) {
	directSet := make(map[string]bool)
	for _, dep := range manifest.Dependencies {
		directSet[dep.String()] = true
	}

	var indirect []domain.Namespace
	for _, dep := range deptreeResult {
		if dep == ns || directSet[dep.String()] {
			continue
		}
		indirect = append(indirect, dep)
	}

	_, _ = svc.vault.PutArrow(ctx, ns, &manifest, indirect)
}

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

	_, workDir, _ := svc.vault.GetArrow(ctx, ns)

	reporter := stepreporter.New(svc.asynxRuntime, ns, 0)

	req := wizard.RunRequest{
		Namespace: ns,
		Variables: vars,
		Steps:     steps,
		WorkDir:   workDir,
	}

	execErr := svc.wizard.Execute(ctx, req, reporter)
	outcome := svc.mapOutcome(execErr)
	_ = svc.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
		Namespace: ns,
		Outcome:   outcome,
	})

	if outcome != domainRuntime.ExecutionOutcomeSuccess {
		return
	}

	entry, _, err := svc.vault.GetArrow(ctx, ns)
	if err != nil {
		_ = svc.vault.DeleteArrow(ctx, ns)
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
		depEntry, _, depErr := svc.vault.GetArrow(resolveCtx, depNs)
		if depErr != nil || depEntry == nil {
			return nil, nil
		}
		return depEntry.Manifest.Dependencies, nil
	}

	topoOrder, err := svc.deptree(ctx, ns, deptree.ResolverFunc(vaultResolver))
	if err != nil {
		_ = svc.vault.DeleteArrow(ctx, ns)
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
		_ = svc.vault.DeleteArrow(ctx, dep)
	}

	_ = svc.vault.DeleteArrow(ctx, ns)
}

func (svc *arrowService) hasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	arrows, err := svc.catalog.List()
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

		entry, _, err := svc.vault.GetArrow(ctx, arrow.Namespace)
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
