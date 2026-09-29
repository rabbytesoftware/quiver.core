package runtimeinternal

import (
	"context"
	"time"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// DrainExecution exposes drainExecution for tests. The badge reconcile is
// variadic so the tests that predate it read the same as they always did.
func DrainExecution(
	ctx context.Context,
	exec wizardPkg.Execution,
	ns string,
	executionID string,
	method string,
	markInstalled func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	markUninstalled func(ctx context.Context, ns domain.Namespace) error,
	markLastUsed func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	reconcileVersionBadge ...func(ctx context.Context, ns domain.Namespace) error,
) {
	hooks := CatalogHooks{
		MarkInstalled:   markInstalled,
		MarkUninstalled: markUninstalled,
		MarkLastUsed:    markLastUsed,
	}
	if len(reconcileVersionBadge) > 0 {
		hooks.ReconcileVersionBadge = reconcileVersionBadge[0]
	}

	drainExecution(ctx, exec, ns, executionID, method, hooks, axRuntime)
}

// SendRecoverInterrupted exposes sendRecoverInterrupted for tests.
func SendRecoverInterrupted(
	ctx context.Context,
	ns domain.Namespace,
	from domain.ArrowState,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	sendRecoverInterrupted(ctx, ns, from, axRuntime)
}

// RecoverRunning exposes recoverRunning for tests.
func RecoverRunning(
	ctx context.Context,
	ns domain.Namespace,
	rt domainRuntime.ArrowRuntime,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
) {
	recoverRunning(ctx, ns, rt, axRuntime, w)
}
