package runner

import (
	"context"
	"errors"
	"maps"
	"strconv"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrStateViolation = errors.New("state violation")
)

// Runner owns the core execution lifecycle for an arrow.
type Runner interface {
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		userVars map[string]string,
	) error
	ExecuteSync(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		userVars map[string]string,
	) error
	Stop(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

// PostExecutionFn is called after ExecuteSync completes, whether or not it errored.
type PostExecutionFn func(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	execErr error,
	outcome domainRuntime.ExecutionOutcome,
)

// HookableRunner extends Runner with a post-execution hook for projections.
type HookableRunner interface {
	Runner
	SetPostExecutionHook(fn PostExecutionFn)
}

type runnerService struct {
	axArrow   asynx.Asynx[domain.Arrow]
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	vault     vault.Vault
	manifold  manifold.Manifold
	netbridge netbridge.Netbridge // may be nil
	wizard    wizard.Wizard       // may be nil
	os        domain.OS
	hook      PostExecutionFn // may be nil
}

// NewRunner constructs a HookableRunner.
func NewRunner(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
	m manifold.Manifold,
	nb netbridge.Netbridge,
	wiz wizard.Wizard,
	os domain.OS,
) HookableRunner {
	return &runnerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
		manifold:  m,
		netbridge: nb,
		wizard:    wiz,
		os:        os,
	}
}

func (r *runnerService) SetPostExecutionHook(fn PostExecutionFn) {
	r.hook = fn
}

func (r *runnerService) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	arrow, err := r.axArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	steps, availableIn := r.stepsForMethod(arrow, method)

	vars, err := r.resolveVariables(ctx, ns, &arrow.Manifest, method, userVars)
	if err != nil {
		return err
	}

	_, sendErr := r.axRuntime.Send(ctx, arrowcmds.BeginExecution{
		Namespace:   ns,
		Method:      method,
		AvailableIn: availableIn,
		Steps:       steps,
		Variables:   vars,
	})
	return sendErr
}

func (r *runnerService) ExecuteSync(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	arrow, err := r.axArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	steps, availableIn := r.stepsForMethod(arrow, method)

	vars, err := r.resolveVariables(ctx, ns, &arrow.Manifest, method, userVars)
	if err != nil {
		return err
	}

	_, err = r.axRuntime.SendWait(ctx, arrowcmds.BeginExecution{
		Namespace:   ns,
		Method:      method,
		AvailableIn: availableIn,
		Steps:       steps,
		Variables:   vars,
	})
	if err != nil {
		return err
	}

	rt, err := r.axRuntime.Get(ctx, ns.String())
	if err != nil {
		return err
	}
	if rt.LastReturn == nil {
		return errors.New("executeSync: execution completed without a result")
	}
	return r.mapOutcomeToError(rt.LastReturn.Outcome)
}

func (r *runnerService) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	runtime, err := r.axRuntime.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return errors.Join(ErrNotFound)
		}
		return err
	}

	if runtime.State != domain.ArrowStateRunning {
		return ErrStateViolation
	}

	if _, err := r.axRuntime.Send(ctx, arrowcmds.MarkStopping{Namespace: ns}); err != nil {
		return err
	}

	return nil
}

func (r *runnerService) resolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.ArrowManifest, string, error) {
	entry, homePath, err := r.vault.GetArrow(ctx, ns)

	if err == nil {
		return entry.Manifest, homePath, nil
	}

	if errors.Is(err, vault.ErrStale) {
		manifest, manifoldErr := r.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return entry.Manifest, homePath, nil
		}

		newPath, putErr := r.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, "", errors.Join(putErr)
		}

		return manifest, newPath, nil
	}

	if errors.Is(err, vault.ErrNotCached) {
		manifest, manifoldErr := r.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return nil, "", manifoldErr
		}

		newPath, putErr := r.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, "", putErr
		}

		return manifest, newPath, nil
	}

	return nil, "", err
}

// resolveVariables builds the variable map for an execution using 6 priority layers:
// built-ins → dep built-ins → manifest defaults → netbridge ports → stored vars → user vars.
func (r *runnerService) resolveVariables(
	ctx context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
	method string,
	userVars map[string]string,
) (map[string]string, error) {
	vars := make(map[string]string)

	// Layer 1: built-ins
	if entry, homePath, err := r.vault.GetArrow(ctx, ns); err == nil && entry != nil {
		vars["INSTALL_PATH"] = homePath
	}
	vars["ARROW_NAMESPACE"] = ns.String()
	vars["PLATFORM"] = r.os.String()

	// Layer 2: dep built-ins
	for _, dep := range manifest.Dependencies {
		if entry, homePath, err := r.vault.GetArrow(ctx, dep); err == nil && entry != nil {
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
	if r.netbridge != nil {
		for _, port := range manifest.Netbridge {
			allocated, err := r.netbridge.Allocate(ctx, ns.String(), port.Protocol, port.Default)
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
	runtime, err := r.axRuntime.Get(ctx, ns.String())
	if err == nil && runtime.LastReturn != nil {
		maps.Copy(vars, runtime.LastReturn.Variables)
	}

	// Layer 6: user vars (highest priority)
	maps.Copy(vars, userVars)

	return vars, nil
}

func (r *runnerService) stepsForMethod(
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

func (r *runnerService) mapOutcomeToError(
	outcome domainRuntime.ExecutionOutcome,
) error {
	switch outcome {
	case domainRuntime.ExecutionOutcomeSuccess:
		return nil
	case domainRuntime.ExecutionOutcomeCancelled:
		return context.Canceled
	default:
		return errors.New("execution failed")
	}
}
