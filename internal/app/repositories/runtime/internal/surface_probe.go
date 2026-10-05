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
// soon as executionID is no longer the aggregate's current execution.
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
	for executionCurrent(ctx, axRuntime, ns, executionID) {
		if hooks.SurfaceReady(ctx, domain.Namespace(ns), s) {
			s.Ready = true
			recordReady(ctx, axRuntime, ns, executionID, s)
			return
		}
		time.Sleep(wait)
		wait = nextProbeInterval(wait, maxInterval)
	}
	slog.DebugContext(ctx, "runtime: surface probe stopped, execution gone", "ns", ns)
}

func nextProbeInterval(current, ceiling time.Duration) time.Duration {
	return min(current*2, ceiling)
}

func executionCurrent(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
) bool {
	rt, err := axRuntime.Get(ctx, ns)
	return err == nil && rt.Execution != nil && rt.Execution.ID == executionID
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
	_, err := axRuntime.Send(ctx, runtimecmds.SetSurface{
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
	_, err := axRuntime.Send(ctx, runtimecmds.SetSurface{
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
