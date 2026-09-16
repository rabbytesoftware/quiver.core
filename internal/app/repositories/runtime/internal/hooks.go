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
	markInstalled func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	markUninstalled func(ctx context.Context, ns domain.Namespace) error,
	markLastUsed func(ctx context.Context, ns domain.Namespace, at time.Time) error,
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
	onEnd(ctx, markInstalled, markUninstalled, markLastUsed, axRuntime, ns, executionID, method, outcome)
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

func sendEndExecution(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	outcome domainRuntime.ExecutionOutcome,
) {
	_, err := axRuntime.Send(ctx, runtimecmds.EndExecution{
		Namespace:   domain.Namespace(ns),
		ExecutionID: executionID,
		Outcome:     outcome,
	})
	if err == nil {
		return
	}
	if errors.Is(err, apperrors.ErrExecutionSuperseded) {
		slog.DebugContext(ctx, "runtime: end dropped, execution superseded", "ns", ns, "outcome", outcome)
		return
	}
	slog.ErrorContext(ctx, "runtime: sendEndExecution failed", "ns", ns, "outcome", outcome, "err", err)
}

func onEnd(
	ctx context.Context,
	markInstalled func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	markUninstalled func(ctx context.Context, ns domain.Namespace) error,
	markLastUsed func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	method string,
	outcome domainRuntime.ExecutionOutcome,
) {
	if outcome == domainRuntime.ExecutionOutcomeSuccess {
		stampCatalog(ctx, markInstalled, markUninstalled, markLastUsed, ns, method)
	}
	sendEndExecution(ctx, axRuntime, ns, executionID, outcome)
}

// stampCatalog records on the arrow what the lifecycle that just succeeded did
// to the disk: an install stamps the moment its ref landed there, an uninstall
// takes that stamp back off, and an execute stamps the moment the arrow was
// last run. Nothing else clears the install stamp, so an arrow that skipped
// this would keep reporting an install it no longer has.
//
// All writes happen before EndExecution commits, for the reason
// repositories/container.go gives: a shutdown that loses this write must lose
// the runtime transition with it, so recovery re-drives the pair.
func stampCatalog(
	ctx context.Context,
	markInstalled func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	markUninstalled func(ctx context.Context, ns domain.Namespace) error,
	markLastUsed func(ctx context.Context, ns domain.Namespace, at time.Time) error,
	ns string,
	method string,
) {
	nsVal := domain.Namespace(ns)

	switch method {
	case domain.MethodInstall:
		if err := markInstalled(ctx, nsVal, time.Now().UTC()); err != nil {
			slog.ErrorContext(ctx, "runtime: MarkInstalled failed", "ns", ns, "err", err)
		}
	case domain.MethodUninstall:
		if err := markUninstalled(ctx, nsVal); err != nil {
			slog.ErrorContext(ctx, "runtime: MarkUninstalled failed", "ns", ns, "err", err)
		}
	case domain.MethodExecute:
		if err := markLastUsed(ctx, nsVal, time.Now().UTC()); err != nil {
			slog.ErrorContext(ctx, "runtime: MarkLastUsed failed", "ns", ns, "err", err)
		}
	}
}
