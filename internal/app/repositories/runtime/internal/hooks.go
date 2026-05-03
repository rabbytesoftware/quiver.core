package runtimeinternal

import (
	"context"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	runtimecmds "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func drainExecution(
	ctx context.Context,
	exec wizardPkg.Execution,
	ns string,
	method string,
	markInstalled func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	for evt := range exec.Events() {
		switch evt.Kind {
		case wizardPkg.EventKindStepStarted:
			sendStep(ctx, axRuntime, ns, evt.StepIndex, domainRuntime.StepStatusRunning, nil)
		case wizardPkg.EventKindStepCompleted:
			sendStep(ctx, axRuntime, ns, evt.StepIndex, domainRuntime.StepStatusCompleted, nil)
		case wizardPkg.EventKindStepFailed:
			sendStep(ctx, axRuntime, ns, evt.StepIndex, domainRuntime.StepStatusFailed, evt.Err)
		case wizardPkg.EventKindPID:
			sendPID(ctx, axRuntime, ns, evt.PID)
		}
	}
	// onEnd fires AFTER the loop — exec.Outcome() is authoritative.
	outcome := exec.Outcome()
	onEnd(ctx, markInstalled, axRuntime, ns, method, outcome)
}

func sendStep(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	stepIndex int,
	status domainRuntime.StepStatus,
	stepErr error,
) {
	var errStr *string
	if stepErr != nil {
		s := stepErr.Error()
		errStr = &s
	}
	if _, err := axRuntime.Send(ctx, runtimecmds.AdvanceStep{
		Namespace: domain.Namespace(ns),
		StepIndex: stepIndex,
		ToStatus:  status,
		Error:     errStr,
	}); err != nil {
		slog.ErrorContext(ctx, "runtime: sendStep failed", "ns", ns, "step", stepIndex, "err", err)
	}
}

func sendPID(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	pid int,
) {
	if _, err := axRuntime.Send(ctx, runtimecmds.RecordPID{
		Namespace: domain.Namespace(ns),
		PID:       pid,
	}); err != nil {
		slog.ErrorContext(ctx, "runtime: sendPID failed", "ns", ns, "pid", pid, "err", err)
	}
}

func sendEndExecution(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	outcome domainRuntime.ExecutionOutcome,
) {
	if _, err := axRuntime.Send(ctx, runtimecmds.EndExecution{
		Namespace: domain.Namespace(ns),
		Outcome:   outcome,
	}); err != nil {
		slog.ErrorContext(ctx, "runtime: sendEndExecution failed", "ns", ns, "outcome", outcome, "err", err)
	}
}

func onEnd(
	ctx context.Context,
	markInstalled func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	method string,
	outcome domainRuntime.ExecutionOutcome,
) {
	if method == domain.MethodInstall && outcome == domainRuntime.ExecutionOutcomeSuccess {
		nsVal := domain.Namespace(ns)
		if err := markInstalled(ctx, nsVal, nsVal.Ref(), time.Now().UTC()); err != nil {
			slog.ErrorContext(ctx, "runtime: MarkInstalled failed", "ns", ns, "err", err)
		}
	}
	sendEndExecution(ctx, axRuntime, ns, outcome)
}
