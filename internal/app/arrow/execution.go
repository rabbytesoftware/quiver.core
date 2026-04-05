package arrow

import (
	"context"
	"errors"
	"maps"
	"strconv"

	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func (svc *arrowService) resolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.ArrowManifest, string, error) {
	entry, homePath, err := svc.engines.Vault.GetArrow(ctx, ns)

	if err == nil {
		return entry.Manifest, homePath, nil
	}

	if errors.Is(err, vault.ErrStale) {
		manifest, manifoldErr := svc.engines.Manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return entry.Manifest, homePath, nil
		}

		newPath, putErr := svc.engines.Vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, "", errors.Join(putErr)
		}

		return manifest, newPath, nil
	}

	if errors.Is(err, vault.ErrNotCached) {
		manifest, manifoldErr := svc.engines.Manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return nil, "", manifoldErr
		}

		newPath, putErr := svc.engines.Vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, "", putErr
		}

		return manifest, newPath, nil
	}

	return nil, "", err
}

func (svc *arrowService) buildDepResolver() deptree.ResolverFunc {
	return func(ctx context.Context, ns domain.Namespace) ([]domain.Namespace, error) {
		manifest, _, err := svc.resolveManifest(ctx, ns)
		if err != nil {
			return nil, err
		}

		return manifest.Dependencies, nil
	}
}

func (svc *arrowService) beginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) (*wizard.RunRequest, *stepreporter.StepReporter, error) {
	arrow, err := svc.asynxArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	steps, availableIn := svc.stepsForMethod(arrow, method)

	vars, err := svc.resolveVariables(ctx, ns, &arrow.Manifest, method, userVars)
	if err != nil {
		return nil, nil, err
	}

	if err := svc.asynxRuntime.Send(ctx, arrowcmds.BeginExecution{
		Namespace:   ns,
		Method:      method,
		AvailableIn: availableIn,
		Steps:       steps,
		Variables:   vars,
	}); err != nil {
		return nil, nil, err
	}

	_, workDir, _ := svc.engines.Vault.GetArrow(ctx, ns)

	indexOffset := 0
	if method == "_install" {
		indexOffset = 1
	}

	reporter := stepreporter.New(svc.asynxRuntime, ns, indexOffset)

	req := &wizard.RunRequest{
		Namespace: ns,
		Variables: vars,
		Steps:     steps,
		WorkDir:   workDir,
	}

	return req, reporter, nil
}

func (svc *arrowService) executeSync(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	req, reporter, err := svc.beginExecution(ctx, ns, method, userVars)
	if err != nil {
		return err
	}

	execErr := svc.engines.Wizard.Execute(ctx, *req, reporter)
	outcome := svc.mapOutcome(execErr)
	_ = svc.asynxRuntime.Send(ctx, arrowcmds.EndExecution{
		Namespace: ns,
		Outcome:   outcome,
	})
	return execErr
}

func (svc *arrowService) resolveVariables(
	ctx context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
	method string,
	userVars map[string]string,
) (map[string]string, error) {
	vars := make(map[string]string)

	// Layer 1: built-ins
	if entry, homePath, err := svc.engines.Vault.GetArrow(ctx, ns); err == nil && entry != nil {
		vars["INSTALL_PATH"] = homePath
	}
	vars["ARROW_NAMESPACE"] = ns.String()
	vars["PLATFORM"] = svc.os.String()

	// Layer 2: dep built-ins
	for _, dep := range manifest.Dependencies {
		if entry, homePath, err := svc.engines.Vault.GetArrow(ctx, dep); err == nil && entry != nil {
			vars[dep.String()+".INSTALL_PATH"] = homePath
		}
	}

	// Layer 3: manifest defaults
	for _, v := range manifest.Variables {
		if v.Default != "" {
			vars[v.Name] = v.Default
		}
	}

	// Layer 4: netbridge ports
	if svc.engines.Netbridge != nil {
		for _, port := range manifest.Netbridge {
			allocated, err := svc.engines.Netbridge.Allocate(ctx, ns.String(), port.Protocol, port.Default)
			if err != nil {
				if port.Required {
					return nil, err
				}
				continue
			}
			vars[port.Name] = strconv.Itoa(allocated)
		}
	}

	// Layer 5: stored vars from last return
	runtime, err := svc.asynxRuntime.Get(ctx, ns.String())
	if err == nil && runtime.LastReturn != nil {
		maps.Copy(vars, runtime.LastReturn.Variables)
	}

	// Layer 6: user vars
	maps.Copy(vars, userVars)

	return vars, nil
}

func (svc *arrowService) handleExecutionError(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	err error,
) {
	if method != "_execute" {
		return
	}
	if !errors.Is(err, context.Canceled) {
		return
	}

	arrow, getErr := svc.asynxArrow.Get(ctx, ns.String())
	if getErr != nil {
		return
	}

	if arrow.Manifest.Lifecycle.Stop == nil {
		return
	}

	go func() {
		_ = svc.BeginExecution(context.Background(), ns, "_stop", nil)
	}()
}

func (svc *arrowService) mapOutcome(err error) domainRuntime.ExecutionOutcome {
	if err == nil {
		return domainRuntime.ExecutionOutcomeSuccess
	}
	if errors.Is(err, context.Canceled) {
		return domainRuntime.ExecutionOutcomeCancelled
	}
	return domainRuntime.ExecutionOutcomeFailed
}

func (svc *arrowService) stepsForMethod(
	arrow domain.Arrow,
	method string,
) ([]domainStep.Step, []domain.ArrowState) {
	switch method {
	case "_install":
		depStep := domainStep.NewDependenciesStep("Resolve dependencies")
		installSteps := []domainStep.Step{depStep}
		installSteps = append(installSteps, arrow.Manifest.Lifecycle.Install...)
		return installSteps, nil
	case "_uninstall":
		return arrow.Manifest.Lifecycle.Uninstall, []domain.ArrowState{domain.ArrowStateReady}
	case "_execute":
		return arrow.Manifest.Lifecycle.Execute, []domain.ArrowState{domain.ArrowStateReady}
	case "_stop":
		return arrow.Manifest.Lifecycle.Stop, []domain.ArrowState{domain.ArrowStateReady}
	default:
		m := arrow.Manifest.Methods[method]
		return m.Steps, m.AvailableIn
	}
}
