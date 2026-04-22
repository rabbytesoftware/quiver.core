package execution

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/installer"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/runner"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	engine "github.com/rabbytesoftware/quiver/internal/engine"
)

type Execution interface {
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		triggeredBy domain.Namespace,
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

// SyncExecutor exposes synchronous execution for use by the builder layer.
type SyncExecutor interface {
	ExecuteSync(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
}

type executionService struct {
	runner    runner.Runner
	installer installer.Installer
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	engines engine.Container,
	os domain.OS,
	onUninstallSuccess func(context.Context, domain.Namespace),
) (Execution, error) {
	run, err := runner.New(
		axArrow,
		axRuntime,
		engines.Vault,
		engines.Netbridge,
		engines.Wizard,
		os,
	)
	if err != nil {
		return nil, err
	}

	inst, err := installer.New(
		axArrow,
		axRuntime,
		engines.Vault,
		run,
	)
	if err != nil {
		return nil, err
	}

	svc := &executionService{runner: run, installer: inst}

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
				_ = run.BeginExecution(ctx, ns, domain.Namespace(""), domain.MethodStop, nil)
			}

		case domain.MethodInstall:
			if outcome == domainRuntime.ExecutionOutcomeSuccess {
				if _, err := axArrow.Send(ctx, arrowcmds.MarkInstalled{
					Namespace:    ns,
					InstalledAt:  time.Now(),
					InstalledRef: ns.Ref(),
				}); err != nil {
					slog.WarnContext(ctx, "mark installed failed", "ns", ns, "err", err)
				}
			}

		case domain.MethodUninstall:
			if outcome != domainRuntime.ExecutionOutcomeSuccess {
				break
			}
			if onUninstallSuccess != nil {
				onUninstallSuccess(ctx, ns)
			}

		}
	})

	return svc, nil
}

func (e *executionService) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	triggeredBy domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	return e.runner.BeginExecution(
		ctx,
		ns,
		triggeredBy,
		method,
		userVars,
	)
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

func (e *executionService) ExecuteSync(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error {
	return e.runner.ExecuteSync(ctx, ns, method, vars)
}
