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
)

const (
	surfaceProbeInterval = 250 * time.Millisecond
	surfaceProbeTimeout  = 60 * time.Second
)

// probeSurface polls until the surface answers, then records Ready. It gives
// up quietly after timeout: the shell keeps showing "starting".
func probeSurface(
	ctx context.Context,
	hooks CatalogHooks,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns string,
	executionID string,
	s domainRuntime.Surface,
	interval time.Duration,
	timeout time.Duration,
) {
	if s.Ready || hooks.SurfaceReady == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		if hooks.SurfaceReady(ctx, domain.Namespace(ns), s) {
			s.Ready = true
			sendSurface(ctx, axRuntime, ns, executionID, s)
			return
		}
		select {
		case <-deadline.C:
			slog.WarnContext(ctx, "runtime: surface never became ready", "ns", ns)
			return
		case <-tick.C:
		}
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
