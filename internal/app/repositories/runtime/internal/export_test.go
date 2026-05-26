package runtimeinternal

import (
	"context"
	"time"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

func DrainExecution(
	ctx context.Context,
	exec wizardPkg.Execution,
	ns string,
	method string,
	markInstalled func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	drainExecution(ctx, exec, ns, method, markInstalled, axRuntime)
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
