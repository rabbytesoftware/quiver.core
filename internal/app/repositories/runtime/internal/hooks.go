package runtimeinternal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// CatalogHooks are the arrow-repository calls the runtime makes as an execution
// ends. They are handed over as closures rather than reached through the arrow
// repository directly because runtime.New itself is constructed with the arrow
// repository's own functions — neither repository can be built first.
//
// Every one of them runs on the drain goroutine (see onBegun), which belongs to
// no asynx worker pool. That is what makes it safe for them to block on the
// arrow aggregate: the cross-instance circular wait documented on
// internal/app/container.go's newAsynx needs an asynx worker blocked on a send
// into another instance, and this deliberately is not one.
type CatalogHooks struct {
	MarkInstalled   func(ctx context.Context, ns domain.Namespace, at time.Time) error
	MarkUninstalled func(ctx context.Context, ns domain.Namespace) error
	MarkLastUsed    func(ctx context.Context, ns domain.Namespace, at time.Time) error
	// ReconcileVersionBadge re-derives the runtime state the badge is read
	// from out of the catalog fact it projects. See reconcileVersionBadge.
	ReconcileVersionBadge func(ctx context.Context, ns domain.Namespace) error
}

// drainExecution translates one wizard execution's events into commands on the
// runtime aggregate. It writes only while the aggregate is still running that
// execution: a stop or an update may take the arrow over mid-run, and from that
// point the events of the superseded run describe a run nobody is waiting on.
// Writing them anyway would land another execution's step progress — and its
// end — on the one that replaced it.
func drainExecution(
	ctx context.Context,
	exec wizardPkg.Execution,
	ns string,
	executionID string,
	method string,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	superseded := false
	for evt := range exec.Events() {
		if superseded {
			continue
		}
		switch evt.Kind {
		case wizardPkg.EventKindStepStarted:
			superseded = sendStep(ctx, axRuntime, ns, executionID, evt.StepIndex, domainRuntime.StepStatusRunning, nil)
		case wizardPkg.EventKindStepCompleted:
			superseded = sendStep(ctx, axRuntime, ns, executionID, evt.StepIndex, domainRuntime.StepStatusCompleted, nil)
		case wizardPkg.EventKindStepFailed:
			superseded = sendStep(ctx, axRuntime, ns, executionID, evt.StepIndex, domainRuntime.StepStatusFailed, evt.Err)
		case wizardPkg.EventKindPID:
			superseded = sendPID(ctx, axRuntime, ns, executionID, evt.PID)
		case wizardPkg.EventKindEnded:
		}
	}
	if superseded {
		return
	}
	// onEnd fires AFTER the loop — exec.Outcome() is authoritative.
	outcome := exec.Outcome()
	if !onEnd(ctx, hooks, axRuntime, ns, executionID, method, outcome) {
		return
	}
	reconcileVersionBadge(ctx, hooks, ns)
}

// reconcileVersionBadge re-derives the outdated badge after an execution
// ends. EndExecution puts the arrow back at ready without consulting
// anything else, so an arrow whose catalog record still says a newer
// release exists reads ready until the next TTL-gated version check, up to
// an hour later.
//
// A re-derivation, not a fresh check: Arrow.Outdated already holds the
// answer and nothing it depends on can change mid-execution, so this skips
// the network round trip and just re-projects it onto runtime state.
//
// Runs unconditionally, regardless of outcome or method: the aggregate read
// is its own short-circuit. Called only after EndExecution's Send returns,
// so the dispatcher's per-aggregate ordering puts the ready broadcast before
// the outdated one that follows it.
func reconcileVersionBadge(
	ctx context.Context,
	hooks CatalogHooks,
	ns string,
) {
	if hooks.ReconcileVersionBadge == nil {
		return
	}
	if err := hooks.ReconcileVersionBadge(ctx, domain.Namespace(ns)); err != nil {
		slog.WarnContext(ctx, "runtime: reconcile version badge after execution", "ns", ns, "err", err)
	}
}

// sendStep reports whether the aggregate has moved on to another execution.
func sendStep(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	stepIndex int,
	status domainRuntime.StepStatus,
	stepErr error,
) bool {
	var errStr *string
	if stepErr != nil {
		s := stepErr.Error()
		errStr = &s
	}
	_, err := axRuntime.Send(ctx, runtimecmds.AdvanceStep{
		Namespace:   domain.Namespace(ns),
		ExecutionID: executionID,
		StepIndex:   stepIndex,
		ToStatus:    status,
		Error:       errStr,
	})
	if err == nil {
		return false
	}
	if errors.Is(err, apperrors.ErrExecutionSuperseded) {
		slog.DebugContext(ctx, "runtime: step dropped, execution superseded", "ns", ns, "step", stepIndex)
		return true
	}
	slog.ErrorContext(ctx, "runtime: sendStep failed", "ns", ns, "step", stepIndex, "err", err)
	return false
}

// sendPID reports whether the aggregate has moved on to another execution.
func sendPID(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	pid int,
) bool {
	_, err := axRuntime.Send(ctx, runtimecmds.RecordPID{
		Namespace:   domain.Namespace(ns),
		ExecutionID: executionID,
		PID:         pid,
	})
	if err == nil {
		return false
	}
	if errors.Is(err, apperrors.ErrExecutionSuperseded) {
		slog.DebugContext(ctx, "runtime: pid dropped, execution superseded", "ns", ns, "pid", pid)
		return true
	}
	slog.ErrorContext(ctx, "runtime: sendPID failed", "ns", ns, "pid", pid, "err", err)
	return false
}

// sendEndExecution reports whether the end actually landed on the aggregate.
// A superseded end, and a send that failed outright, both leave the runtime
// describing something other than the execution that just finished, so nothing
// downstream of the end should read the aggregate as if it had moved.
func sendEndExecution(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	outcome domainRuntime.ExecutionOutcome,
) bool {
	_, err := axRuntime.Send(ctx, runtimecmds.EndExecution{
		Namespace:   domain.Namespace(ns),
		ExecutionID: executionID,
		Outcome:     outcome,
	})
	if err == nil {
		return true
	}
	if errors.Is(err, apperrors.ErrExecutionSuperseded) {
		slog.DebugContext(ctx, "runtime: end dropped, execution superseded", "ns", ns, "outcome", outcome)
		return false
	}
	slog.ErrorContext(ctx, "runtime: sendEndExecution failed", "ns", ns, "outcome", outcome, "err", err)
	return false
}

// onEnd reports whether EndExecution committed.
func onEnd(
	ctx context.Context,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	method string,
	outcome domainRuntime.ExecutionOutcome,
) bool {
	if outcome == domainRuntime.ExecutionOutcomeSuccess {
		stampCatalog(ctx, hooks, ns, method)
	}
	return sendEndExecution(ctx, axRuntime, ns, executionID, outcome)
}

// stampCatalog records what a succeeded lifecycle did to disk: install
// stamps the ref landing, uninstall clears it, execute stamps last-used.
// Writes happen before EndExecution commits so a lost shutdown loses both
// together (see repositories/container.go).
func stampCatalog(
	ctx context.Context,
	hooks CatalogHooks,
	ns string,
	method string,
) {
	nsVal := domain.Namespace(ns)

	switch method {
	case domain.MethodInstall:
		if err := hooks.MarkInstalled(ctx, nsVal, time.Now().UTC()); err != nil {
			slog.ErrorContext(ctx, "runtime: MarkInstalled failed", "ns", ns, "err", err)
		}
	case domain.MethodUninstall:
		if err := hooks.MarkUninstalled(ctx, nsVal); err != nil {
			slog.ErrorContext(ctx, "runtime: MarkUninstalled failed", "ns", ns, "err", err)
		}
	case domain.MethodExecute:
		if err := hooks.MarkLastUsed(ctx, nsVal, time.Now().UTC()); err != nil {
			slog.ErrorContext(ctx, "runtime: MarkLastUsed failed", "ns", ns, "err", err)
		}
	}
}
