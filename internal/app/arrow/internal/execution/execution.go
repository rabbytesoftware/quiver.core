package execution

import (
	"context"
	"errors"

	"github.com/char2cs/asynx"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/installer"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/installer/deps"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/runner"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	engine "github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// Execution coordinates the execution lifecycle for an arrow.
type Execution interface {
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
}

type executionService struct {
	runner    runner.Runner
	installer installer.Installer
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	engines *engine.Container,
	os domain.OS,
	cat catalog.Catalog,
) (Execution, error) {
	run, err := runner.New(axArrow, axRuntime, engines.Vault, engines.Netbridge, engines.Wizard, os)
	if err != nil {
		return nil, err
	}

	dep := deps.New(engines.DepTree, engines.Vault, engines.Manifold, axArrow, axRuntime, run)
	if engines.Wizard != nil {
		engines.Wizard.RegisterDispatch(
			domainstep.StepTypeDependencies,
			wizard.Adapt[domainstep.DependenciesStep](dep),
		)
	}

	inst, err := installer.New(axArrow, axRuntime, engines.Vault, engines.DepTree, cat, run)
	if err != nil {
		return nil, err
	}

	svc := &executionService{runner: run, installer: inst}

	// Wire post-execution hook — replaces SetService + SetSyncInstall pattern.
	// The hook uses svc.installer so tests can substitute the installer after construction.
	run.SetPostExecutionHook(func(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		execErr error,
		outcome domainRuntime.ExecutionOutcome,
	) {
		switch method {
		case domain.MethodExecute:
			if errors.Is(execErr, context.Canceled) {
				_ = run.BeginExecution(ctx, ns, domain.MethodStop, nil)
			}
		case domain.MethodUninstall:
			if outcome == domainRuntime.ExecutionOutcomeSuccess {
				svc.installer.CleanupAfterUninstall(ctx, ns)
			}
		}
	})

	return svc, nil
}

func (e *executionService) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	return e.runner.BeginExecution(ctx, ns, method, userVars)
}

func (e *executionService) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return e.runner.Stop(ctx, ns)
}

func (e *executionService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	return e.installer.Install(ctx, ns, userVars)
}

func (e *executionService) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	return e.installer.Uninstall(ctx, ns, userVars)
}
