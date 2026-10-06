package runtimeinternal

import (
	"context"
	"log/slog"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// RecoveryOption adjusts what RecoverTransients does beyond the runtime store.
type RecoveryOption func(*recoveryOptions)

type recoveryOptions struct {
	closeSurface func(ns domain.Namespace)
}

// WithCloseSurface releases what a surface held (its socket file) for a run
// recovery finds dead. Nothing else would: the execution that opened it ended
// with the daemon that ran it.
func WithCloseSurface(
	closeSurface func(ns domain.Namespace),
) RecoveryOption {
	return func(o *recoveryOptions) {
		o.closeSurface = closeSurface
	}
}

func RecoverTransients(
	ctx context.Context,
	listArrows func(ctx context.Context) ([]models.ArrowView, error),
	listRuntimeAggregates func(ctx context.Context) ([]domain.Namespace, error),
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	opts ...RecoveryOption,
) {
	var o recoveryOptions
	for _, opt := range opts {
		opt(&o)
	}
	for _, ns := range collectRecoveryNamespaces(ctx, listArrows, listRuntimeAggregates) {
		if preloadErr := axRuntime.Preload(ctx, ns.String()); preloadErr != nil {
			continue
		}
		rt, getErr := axRuntime.Get(ctx, ns.String())
		if getErr != nil || rt.Ref == "" {
			continue
		}
		switch rt.State {
		case domain.ArrowStateRunning:
			recoverRunning(ctx, ns, rt, axRuntime, w, o.closeSurface)
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

// collectRecoveryNamespaces merges catalog namespaces with runtime-store aggregate
// namespaces, deduplicated. Either source failing is logged and skipped, never fatal.
func collectRecoveryNamespaces(
	ctx context.Context,
	listArrows func(ctx context.Context) ([]models.ArrowView, error),
	listRuntimeAggregates func(ctx context.Context) ([]domain.Namespace, error),
) []domain.Namespace {
	seen := make(map[string]struct{})
	var out []domain.Namespace

	add := func(ns domain.Namespace) {
		key := ns.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, ns)
	}

	if items, err := listArrows(ctx); err != nil {
		slog.WarnContext(ctx, "crash recovery: list catalog", "err", err)
	} else {
		for _, vm := range items {
			for _, ver := range vm.Versions {
				add(ver.Namespace)
			}
		}
	}

	if aggs, err := listRuntimeAggregates(ctx); err != nil {
		slog.WarnContext(ctx, "crash recovery: list runtime store", "err", err)
	} else {
		for _, ns := range aggs {
			add(ns)
		}
	}

	return out
}

func recoverRunning(
	ctx context.Context,
	ns domain.Namespace,
	rt domainRuntime.ArrowRuntime,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	closeSurface func(ns domain.Namespace),
) {
	pid := 0
	if rt.Execution != nil {
		pid = rt.Execution.PID
	}

	if pid > 0 && w.ProcessAlive(pid) {
		cmd := runtimecmds.RecordDetached{Namespace: ns}
		if _, err := axRuntime.SendWait(ctx, cmd); err != nil {
			slog.WarnContext(
				ctx,
				"crash recovery: failed to recover live process",
				"ns", ns,
				"pid", pid,
				"event", cmd.EventName(),
				"err", err,
			)
			return
		}

		slog.InfoContext(
			ctx,
			"crash recovery: recovered live process",
			"ns", ns,
			"pid", pid,
			"event", cmd.EventName(),
		)

		return
	}

	sendRecoverInterrupted(
		ctx,
		ns,
		domain.ArrowStateRunning,
		axRuntime,
	)
	if closeSurface != nil && rt.Execution != nil && rt.Execution.Surface != nil {
		closeSurface(ns)
	}
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
