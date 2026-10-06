package runtimeinternal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const (
	surfaceProbeInterval    = 250 * time.Millisecond
	surfaceProbeMaxInterval = 2 * time.Second
)

// probeSurface polls until the surface answers, then records Ready. It has no
// deadline of its own: a slow first start still becomes ready. It stops as
// soon as s is no longer the open surface of executionID: the run that opened
// it exited, or the aggregate moved on to another execution.
func probeSurface(
	ctx context.Context,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	s domainRuntime.Surface,
	interval time.Duration,
	maxInterval time.Duration,
) {
	if s.Ready || hooks.SurfaceReady == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	wait := interval
	for surfaceOpen(ctx, axRuntime, ns, executionID, s) {
		if hooks.SurfaceReady(ctx, domain.Namespace(ns), s) {
			s.Ready = true
			recordReady(ctx, axRuntime, ns, executionID, s)
			return
		}
		time.Sleep(wait)
		wait = nextProbeInterval(wait, maxInterval)
	}
	slog.DebugContext(ctx, "runtime: surface probe stopped, surface gone", "ns", ns)
}

func nextProbeInterval(
	current time.Duration,
	ceiling time.Duration,
) time.Duration {
	return min(current*2, ceiling)
}

// surfaceOpen reports whether s is still the surface executionID has open,
// ignoring Ready: a later run of the same execution may have opened another
// one, which this probe must leave alone.
func surfaceOpen(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	s domainRuntime.Surface,
) bool {
	rt, err := axRuntime.Get(ctx, ns)
	if err != nil || rt.Execution == nil || rt.Execution.ID != executionID || rt.Execution.Surface == nil {
		return false
	}
	open := *rt.Execution.Surface
	open.Ready, s.Ready = false, false
	return open == s
}

// recordReady treats every rejection that means the execution is gone (ended,
// replaced or the store shutting down) as routine.
func recordReady(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	s domainRuntime.Surface,
) {
	err := sendRetryingConflicts(ctx, axRuntime, runtimecmds.SetSurface{
		Namespace:   domain.Namespace(ns),
		ExecutionID: executionID,
		Surface:     s,
	})
	switch {
	case err == nil:
	case errors.Is(err, asynxModels.ErrValidation), errors.Is(err, asynxModels.ErrShuttingDown):
		slog.DebugContext(ctx, "runtime: surface ready dropped, execution gone", "ns", ns, "err", err)
	default:
		slog.ErrorContext(ctx, "runtime: recording surface ready failed", "ns", ns, "err", err)
	}
}

// sendSurface reports whether the aggregate has moved on to another execution.
func sendSurface(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	s domainRuntime.Surface,
) bool {
	err := sendRetryingConflicts(ctx, axRuntime, runtimecmds.SetSurface{
		Namespace:   domain.Namespace(ns),
		ExecutionID: executionID,
		Surface:     s,
	})
	if err == nil {
		return false
	}
	if errors.Is(err, apperrors.ErrExecutionSuperseded) {
		slog.DebugContext(ctx, "runtime: surface dropped, execution superseded", "ns", ns)
		return true
	}
	slog.ErrorContext(ctx, "runtime: sendSurface failed", "ns", ns, "err", err)
	return false
}

// closeSurface clears the surface of executionID and reports whether the
// aggregate has moved on to another execution, in which case the surface
// was never this drain's to clear nor its socket to release.
func closeSurface(
	ctx context.Context,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
) bool {
	err := sendRetryingConflicts(ctx, axRuntime, runtimecmds.ClearSurface{
		Namespace:   domain.Namespace(ns),
		ExecutionID: executionID,
	})
	if errors.Is(err, apperrors.ErrExecutionSuperseded) {
		slog.DebugContext(ctx, "runtime: surface clear dropped, execution superseded", "ns", ns)
		return true
	}
	if err != nil {
		slog.ErrorContext(ctx, "runtime: clearing surface failed", "ns", ns, "err", err)
	}
	if hooks.CloseSurface != nil {
		hooks.CloseSurface(domain.Namespace(ns))
	}
	return false
}
