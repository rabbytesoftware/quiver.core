package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// Runner owns the core execution lifecycle for an arrow.
type Runner interface {
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		triggeredBy domain.Namespace,
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
	netbridge netbridge.Netbridge // may be nil
	wizard    wizard.Wizard       // may be nil
	os        domain.OS
	hook      PostExecutionFn // may be nil
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
	nb netbridge.Netbridge,
	wiz wizard.Wizard,
	os domain.OS,
) (HookableRunner, error) {
	r := &runnerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
		netbridge: nb,
		wizard:    wiz,
		os:        os,
	}
	if err := r.registerProjections(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *runnerService) SetPostExecutionHook(fn PostExecutionFn) {
	r.hook = fn
}

func (r *runnerService) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	triggeredBy domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	arrow, err := r.axArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return apperrors.ErrNotFound
		}
		return err
	}

	target, resolvedArrow, err := r.resolveTarget(ctx, &arrow, ns)
	if err != nil {
		return err
	}

	steps, availableIn, err := r.stepsForMethod(target, method)
	if err != nil {
		return err
	}

	vars, err := r.resolveVariables(ctx, ns, resolvedArrow, target, method, userVars)
	if err != nil {
		return err
	}

	if method == domain.MethodExecute {
		for _, edge := range target.Services {
			rt, rtErr := r.axRuntime.Get(ctx, edge.Namespace.String())
			if rtErr != nil && !errors.Is(rtErr, asynxModels.ErrNotFound) {
				return fmt.Errorf("get service dep %s: %w", edge.Namespace, rtErr)
			}
			if rtErr == nil && rt.State.IsActive() {
				continue
			}
			if startErr := r.BeginExecution(ctx, edge.Namespace, domain.Namespace(""), domain.MethodExecute, nil); startErr != nil {
				return fmt.Errorf("start service dep %s: %w", edge.Namespace, startErr)
			}
		}
	}

	_, sendErr := r.axRuntime.Send(ctx, arrowcmds.BeginExecution{
		Namespace:   ns,
		TriggeredBy: triggeredBy,
		Method:      method,
		AvailableIn: availableIn,
		Steps:       steps,
		Variables:   vars,
	})
	if errors.Is(sendErr, asynxModels.ErrValidation) ||
		errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
		return fmt.Errorf("begin execution: %w", apperrors.ErrStateViolation)
	}
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
			return apperrors.ErrNotFound
		}
		return err
	}

	target, resolvedArrow, err := r.resolveTarget(ctx, &arrow, ns)
	if err != nil {
		return err
	}

	steps, availableIn, err := r.stepsForMethod(target, method)
	if err != nil {
		return err
	}

	vars, err := r.resolveVariables(ctx, ns, resolvedArrow, target, method, userVars)
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
		if errors.Is(err, asynxModels.ErrValidation) ||
			errors.Is(err, asynxModels.ErrPipelineFailed) {
			return fmt.Errorf("execute sync: %w", apperrors.ErrStateViolation)
		}
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
			return apperrors.ErrNotFound
		}
		return err
	}

	if runtime.State != domain.ArrowStateRunning && runtime.State != domain.ArrowStateExecuting {
		return apperrors.ErrStateViolation
	}

	if r.wizard != nil {
		go r.wizard.Cancel(ns)
	}

	return nil
}

// resolveTarget reads the OS-selected compiled target from the Arrow aggregate.
func (r *runnerService) resolveTarget(
	ctx context.Context,
	arrow *domain.Arrow,
	ns domain.Namespace,
) (domain.Target, *domain.Arrow, error) {
	target, ok := arrow.Targets[r.os]
	if !ok {
		return domain.Target{}, nil, apperrors.ErrPlatformNotSupported
	}

	return target, arrow, nil
}

// resolveVariables builds the variable map for an execution using 6 priority layers:
// built-ins → dep built-ins + named exports → version defaults → netbridge ports → stored vars → user vars.
func (r *runnerService) resolveVariables(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	target domain.Target,
	method string,
	userVars map[string]string,
) (map[string]string, error) {
	vars := make(map[string]string)

	// Layer 1: built-ins
	if entry, homePath, err := r.vault.GetArrow(ctx, ns); err == nil && entry != nil {
		vars["INSTALL_PATH"] = homePath
		vars["WORKDIR"] = filepath.Dir(homePath)
	}
	vars["ARROW_NAMESPACE"] = ns.String()
	vars["PLATFORM"] = r.os.String()

	// Layer 2: dep built-ins and named exports
	for _, edge := range append(target.Tools, target.Services...) {
		depNs := edge.Namespace.BareNamespace()

		depArrow, err := r.axArrow.Get(ctx, depNs.String())
		if err != nil {
			if !errors.Is(err, asynxModels.ErrNotFound) {
				slog.WarnContext(ctx,
					"resolveVariables: unexpected error fetching dep",
					"dep",
					depNs,
					"err",
					err,
				)
			}
			continue
		}

		depTarget, ok := depArrow.Targets[r.os]
		if !ok {
			continue
		}

		// INSTALL_PATH from vault
		if _, homePath, err := r.vault.GetArrow(ctx, edge.Namespace); err == nil {
			vars[depNs.String()+".INSTALL_PATH"] = homePath
		}

		// Named exports — anchor relative paths to dep's INSTALL_PATH
		installPath := vars[depNs.String()+".INSTALL_PATH"]
		for exportName, exportValue := range depTarget.Exports {
			resolved := exportValue
			if strings.HasPrefix(exportValue, "./") && installPath != "" {
				resolved = filepath.Join(installPath, exportValue)
			}
			vars[depNs.String()+"."+exportName] = resolved
		}
	}

	// Layer 3: arrow defaults
	for _, v := range arrow.Variables {
		if v.Default != "" {
			vars[v.Name] = v.Default
		}
	}

	// Layer 4: netbridge ports
	if r.netbridge != nil {
		for _, port := range arrow.Netbridge {
			allocated, err := r.netbridge.Allocate(
				ctx,
				ns.String(),
				port.Protocol,
				port.Default,
			)
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

	// Validate: variables with no default must be resolved by now.
	for _, v := range arrow.Variables {
		if v.Default == "" {
			if _, ok := vars[v.Name]; !ok {
				return nil, fmt.Errorf("%w: %q", apperrors.ErrMissingVariable, v.Name)
			}
		}
	}

	return vars, nil
}

func (r *runnerService) stepsForMethod(
	target domain.Target,
	method string,
) ([]domainStep.Step, []domain.ArrowState, error) {
	switch method {
	case domain.MethodInstall:
		depStep := domainStep.NewDependenciesStep("Resolve dependencies")
		installSteps := []domainStep.Step{depStep}
		installSteps = append(installSteps, target.Lifecycle.Install...)

		return installSteps, nil, nil

	case domain.MethodUninstall:
		return target.Lifecycle.Uninstall,
			[]domain.ArrowState{domain.ArrowStateReady},
			nil

	case domain.MethodUpdate:
		if len(target.Lifecycle.Update) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}
		return target.Lifecycle.Update,
			[]domain.ArrowState{domain.ArrowStateReady},
			nil

	case domain.MethodExecute:
		if len(target.Lifecycle.Execute) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}

		return target.Lifecycle.Execute, []domain.ArrowState{domain.ArrowStateReady}, nil

	case domain.MethodStop:
		if len(target.Lifecycle.Stop) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}

		return target.Lifecycle.Stop,
			[]domain.ArrowState{domain.ArrowStateReady},
			nil

	default:
		m, ok := target.Methods[method]
		if !ok || len(m.Steps) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}

		return m.Steps,
			m.AvailableIn,
			nil
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
