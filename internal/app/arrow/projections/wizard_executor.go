package projections

import (
	"context"
	"errors"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// SyncExecuteFn executes an arrow method synchronously (used for orphan dep cleanup).
// Injected via SetSyncExecute to break the circular dependency with arrowService.
type SyncExecuteFn func(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error

// TriggerStopFn triggers the _stop lifecycle for a namespace after a cancelled _execute.
// Injected via SetTriggerStop; the closure in builder.go guards on stop lifecycle existence.
type TriggerStopFn func(ctx context.Context, ns domain.Namespace)

// WizardExecutor is a projection that fires wizard.Execute in response to runtime.begun events.
type WizardExecutor struct {
	vault        vault.Vault
	depTree      deptree.DepTree
	asynxArrow   asynx.Asynx[domain.Arrow]
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	catalog      arrowstore.ArrowCatalog
	wizard       wizard.Wizard
	syncExecute  SyncExecuteFn
	triggerStop  TriggerStopFn
}

func NewWizardExecutor(
	v vault.Vault,
	dt deptree.DepTree,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	catalog arrowstore.ArrowCatalog,
	wiz wizard.Wizard,
) *WizardExecutor {
	return &WizardExecutor{
		vault:        v,
		depTree:      dt,
		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
		catalog:      catalog,
		wizard:       wiz,
	}
}

func (e *WizardExecutor) SetSyncExecute(fn SyncExecuteFn) { e.syncExecute = fn }
func (e *WizardExecutor) SetTriggerStop(fn TriggerStopFn) { e.triggerStop = fn }

func (e *WizardExecutor) Handler() func(context.Context, asynxModels.Event[domainRuntime.ArrowRuntime]) {
	return func(ctx context.Context, evt asynxModels.Event[domainRuntime.ArrowRuntime]) {
		e.execute(ctx, evt.Aggregate)
	}
}

func (e *WizardExecutor) execute(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	if rt.ActiveRun == nil {
		return
	}

	ns := rt.Namespace
	method := rt.ActiveRun.Method

	steps := make([]domainstep.Step, len(rt.ActiveRun.Steps))
	for i, sp := range rt.ActiveRun.Steps {
		steps[i] = sp.Step
	}

	_, workDir, _ := e.vault.GetArrow(ctx, ns)

	reporter := stepreporter.New(e.asynxRuntime, ns)
	req := wizard.RunRequest{
		Namespace: ns,
		Variables: rt.ActiveRun.Variables,
		Steps:     steps,
		WorkDir:   workDir,
	}

	execErr := e.wizard.Execute(ctx, req, reporter)

	// executeSync already registered this namespace — it owns EndExecution.
	// Return silently without sending EndExecution.
	if errors.Is(execErr, wizard.ErrExecutionExists) {
		return
	}

	outcome := mapOutcome(execErr)
	_, _ = e.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
		Namespace: ns,
		Outcome:   outcome,
	})

	e.handlePostExecution(ctx, ns, method, execErr, outcome)
}

func mapOutcome(err error) domainRuntime.ExecutionOutcome {
	if err == nil {
		return domainRuntime.ExecutionOutcomeSuccess
	}
	if errors.Is(err, context.Canceled) {
		return domainRuntime.ExecutionOutcomeCancelled
	}
	return domainRuntime.ExecutionOutcomeFailed
}

func (e *WizardExecutor) handlePostExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	execErr error,
	outcome domainRuntime.ExecutionOutcome,
) {
	switch method {
	case "_execute":
		if errors.Is(execErr, context.Canceled) && e.triggerStop != nil {
			e.triggerStop(ctx, ns)
		}
	case "_uninstall":
		if outcome == domainRuntime.ExecutionOutcomeSuccess {
			e.cleanupAfterUninstall(ctx, ns)
		}
	}
}

func (e *WizardExecutor) cleanupAfterUninstall(ctx context.Context, ns domain.Namespace) {
	entry, _, err := e.vault.GetArrow(ctx, ns)
	if err != nil {
		_ = e.vault.DeleteArrow(ctx, ns)
		return
	}

	allDeps := make([]domain.Namespace, 0, len(entry.Manifest.Dependencies)+len(entry.IndirectDependencies))
	allDeps = append(allDeps, entry.Manifest.Dependencies...)
	allDeps = append(allDeps, entry.IndirectDependencies...)

	orphaned := make(map[domain.Namespace]bool)
	for _, dep := range allDeps {
		hasDeps, depErr := e.hasDependentsOf(ctx, dep, ns)
		if depErr != nil || hasDeps {
			continue
		}
		orphaned[dep] = true
	}

	vaultResolver := func(resolveCtx context.Context, depNs domain.Namespace) ([]domain.Namespace, error) {
		depEntry, _, depErr := e.vault.GetArrow(resolveCtx, depNs)
		if depErr != nil || depEntry == nil {
			return nil, nil
		}
		return depEntry.Manifest.Dependencies, nil
	}

	topoOrder, topoErr := e.depTree.Resolve(ctx, ns, deptree.ResolverFunc(vaultResolver))
	if topoErr != nil {
		_ = e.vault.DeleteArrow(ctx, ns)
		return
	}

	for i := len(topoOrder) - 1; i >= 0; i-- {
		dep := topoOrder[i]
		if dep == ns || !orphaned[dep] {
			continue
		}
		if e.syncExecute == nil {
			continue
		}
		if uninstallErr := e.syncExecute(ctx, dep, "_uninstall", nil); uninstallErr != nil {
			continue
		}
		_ = e.vault.DeleteArrow(ctx, dep)
	}

	_ = e.vault.DeleteArrow(ctx, ns)
}

// hasDependentsOf mirrors hasDependentsOf in internal/app/arrow/lifecycle.go.
// Duplicated here because the projections package cannot import the arrow package
// without creating a circular dependency. Keep both in sync if the logic changes.
func (e *WizardExecutor) hasDependentsOf(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	arrows, err := e.catalog.List(ctx)
	if err != nil {
		return false, err
	}

	for _, arrow := range arrows {
		if arrow.Removed || arrow.Namespace == excludeNs || arrow.Namespace == ns {
			continue
		}

		rt, rtErr := e.asynxRuntime.Get(ctx, arrow.Namespace.String())
		if rtErr != nil || rt.State == domain.ArrowStateAbsent || rt.Namespace == "" {
			continue
		}

		entry, _, entryErr := e.vault.GetArrow(ctx, arrow.Namespace)
		if entryErr != nil {
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
