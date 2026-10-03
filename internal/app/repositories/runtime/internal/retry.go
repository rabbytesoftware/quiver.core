package runtimeinternal

import (
	"context"
	"log/slog"
	"reflect"

	"github.com/char2cs/asynx"

	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

func superviseExecution(
	ctx context.Context,
	exec wizardPkg.Execution,
	req wizardPkg.RunRequest,
	executionID string,
	hooks CatalogHooks,
	w wizardPkg.Wizard,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	ns := req.Namespace.String()
	report := drainEvents(ctx, exec, ns, executionID, axRuntime)
	if retried, ok := retryOnChecksumMismatch(ctx, exec, report, req, executionID, hooks, w, axRuntime); ok {
		exec = retried
		report = drainEvents(ctx, exec, ns, executionID, axRuntime)
	}
	finishExecution(ctx, exec, report, ns, executionID, req.Method, hooks, axRuntime)
}

func retryOnChecksumMismatch(
	ctx context.Context,
	exec wizardPkg.Execution,
	report drainReport,
	req wizardPkg.RunRequest,
	executionID string,
	hooks CatalogHooks,
	w wizardPkg.Wizard,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) (wizardPkg.Execution, bool) {
	if !worthRetrying(ctx, exec, report, req.Method, hooks) {
		return nil, false
	}
	steps, vars, ok := refreshedRun(ctx, req, hooks)
	if !ok {
		return nil, false
	}
	if !restart(ctx, axRuntime, req.Namespace, executionID, steps) {
		return nil, false
	}
	slog.InfoContext(ctx, "runtime: checksum mismatch, retrying once with a refreshed manifest", "ns", req.Namespace, "method", req.Method)
	req.Steps = steps
	req.Variables = vars
	return w.Start(ctx, req), true
}

func worthRetrying(
	ctx context.Context,
	exec wizardPkg.Execution,
	report drainReport,
	method string,
	hooks CatalogHooks,
) bool {
	if !report.checksumMismatch || report.superseded || ctx.Err() != nil {
		return false
	}
	if method != domain.MethodInstall && method != domain.MethodUpdate {
		return false
	}
	if hooks.RefreshManifest == nil || hooks.Reassemble == nil {
		return false
	}
	return exec.Outcome() == domainRuntime.ExecutionOutcomeFailed
}

func refreshedRun(
	ctx context.Context,
	req wizardPkg.RunRequest,
	hooks CatalogHooks,
) ([]domainStep.Step, map[string]string, bool) {
	if err := hooks.RefreshManifest(ctx, req.Namespace, req.Method); err != nil {
		slog.WarnContext(ctx, "runtime: refresh manifest after checksum mismatch", "ns", req.Namespace, "err", err)
		return nil, nil, false
	}
	steps, vars, err := hooks.Reassemble(ctx, req.Namespace, req.Method, req.Variables)
	if err != nil {
		slog.WarnContext(ctx, "runtime: reassemble after checksum mismatch", "ns", req.Namespace, "err", err)
		return nil, nil, false
	}
	changed := !reflect.DeepEqual(steps, req.Steps) || !reflect.DeepEqual(vars, req.Variables)
	return steps, vars, changed
}

func restart(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	executionID string,
	steps []domainStep.Step,
) bool {
	_, err := axRuntime.Send(ctx, runtimecmds.RestartExecution{
		Namespace:   ns,
		ExecutionID: executionID,
		Steps:       steps,
	})
	if err == nil {
		return true
	}
	slog.WarnContext(ctx, "runtime: restart execution after checksum mismatch", "ns", ns, "err", err)
	return false
}
