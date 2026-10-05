package runtimeinternal

import (
	"context"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

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

// SuperviseExecution exposes superviseExecution for tests.
func SuperviseExecution(
	ctx context.Context,
	exec wizardPkg.Execution,
	req wizardPkg.RunRequest,
	executionID string,
	hooks CatalogHooks,
	w wizardPkg.Wizard,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	superviseExecution(ctx, exec, req, executionID, hooks, w, axRuntime)
}

// DrainExecutionWithHooks exposes drainExecution with a full set of hooks.
func DrainExecutionWithHooks(
	ctx context.Context,
	exec wizardPkg.Execution,
	ns string,
	executionID string,
	method string,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	drainExecution(ctx, exec, ns, executionID, method, hooks, axRuntime)
}

// DrainEvents exposes drainEvents for tests, reporting whether the run was
// superseded.
func DrainEvents(
	ctx context.Context,
	exec wizardPkg.Execution,
	ns string,
	executionID string,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) bool {
	return drainEvents(ctx, exec, ns, executionID, hooks, axRuntime).superseded
}

// ProbeSurface exposes probeSurface for tests.
func ProbeSurface(
	ctx context.Context,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	s domainRuntime.Surface,
	interval time.Duration,
	maxInterval time.Duration,
) {
	probeSurface(ctx, hooks, axRuntime, ns, executionID, s, interval, maxInterval)
}

// NextProbeInterval exposes nextProbeInterval for tests.
func NextProbeInterval(current, ceiling time.Duration) time.Duration {
	return nextProbeInterval(current, ceiling)
}

// ConflictRetryAttempts exposes conflictRetryAttempts for tests.
const ConflictRetryAttempts = conflictRetryAttempts

// SendRetryingConflicts exposes sendRetryingConflicts for tests.
func SendRetryingConflicts(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	cmd asynxModels.Command[domainRuntime.ArrowRuntime],
) error {
	return sendRetryingConflicts(ctx, axRuntime, cmd)
}
