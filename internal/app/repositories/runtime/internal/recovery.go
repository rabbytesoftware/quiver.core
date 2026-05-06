package runtimeinternal

import (
	"context"
	"log/slog"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	runtimecmds "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func RecoverTransients(
	ctx context.Context,
	listArrows func(ctx context.Context) ([]models.ArrowView, error),
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
) {
	items, err := listArrows(ctx)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: list store", "err", err)
		return
	}
	for _, vm := range items {
		for _, ver := range vm.Versions {
			ns := ver.Namespace
			if preloadErr := axRuntime.Preload(ctx, ns.String()); preloadErr != nil {
				continue
			}
			rt, getErr := axRuntime.Get(ctx, ns.String())
			if getErr != nil || rt.Ref == "" {
				continue
			}
			switch rt.State {
			case domain.ArrowStateRunning:
				recoverRunning(ctx, ns, rt, axRuntime, w)
			case domain.ArrowStateInstalling,
				domain.ArrowStateUninstalling,
				domain.ArrowStateUpdating,
				domain.ArrowStateStopping,
				domain.ArrowStateDraining:
				sendRecoverInterrupted(ctx, ns, rt.State, axRuntime)
			case domain.ArrowStateAbsent,
				domain.ArrowStateReady,
				domain.ArrowStateDetached,
				domain.ArrowStateRemoved,
				domain.ArrowStateOutdated:
			}
		}
	}
}

func recoverRunning(
	ctx context.Context,
	ns domain.Namespace,
	rt domainRuntime.ArrowRuntime,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
) {
	pid := 0
	if rt.Execution != nil {
		pid = rt.Execution.PID
	}

	if pid > 0 && w.ProcessAlive(pid) {
		if _, err := axRuntime.SendWait(
			ctx,
			runtimecmds.RecordDetached{Namespace: ns},
		); err != nil {
			slog.WarnContext(
				ctx,
				"crash recovery: failed to detach",
				"ns", ns,
				"pid", pid,
				"err", err,
			)
			return
		}

		slog.InfoContext(
			ctx,
			"crash recovery: detached",
			"ns", ns,
			"pid", pid,
		)

		return
	}

	sendRecoverInterrupted(
		ctx,
		ns,
		domain.ArrowStateRunning,
		axRuntime,
	)
}

func sendRecoverInterrupted(
	ctx context.Context,
	ns domain.Namespace,
	from domain.ArrowState,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) {
	if _, err := axRuntime.SendWait(
		ctx,
		runtimecmds.RecoverInterrupted{Namespace: ns},
	); err != nil {
		slog.WarnContext(
			ctx,
			"crash recovery: failed to recover",
			"ns", ns,
			"from", from,
			"err", err,
		)

		return
	}
	slog.InfoContext(
		ctx,
		"crash recovery: recovered",
		"ns", ns,
		"from", from,
	)
}
