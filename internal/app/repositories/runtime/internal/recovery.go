package runtimeinternal

import (
	"context"
	"log/slog"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

func RecoverTransients(
	ctx context.Context,
	listArrows func(ctx context.Context) ([]models.ArrowView, error),
	listRuntimeAggregates func(ctx context.Context) ([]domain.Namespace, error),
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
) {
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
) {
	pid := 0
	if rt.Execution != nil {
		pid = rt.Execution.PID
	}

	if pid > 0 && w.ProcessAlive(pid) {
		cmd := recoveryCommandFor(ns, rt)
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
}

// recoveryCommandFor chooses RecordSelfRestored only for quiver.core's own
// self-namespace — every other arrow always gets RecordDetached, preserving
// today's "user must stop and restart to restore monitoring" contract. A
// process surviving alongside a crashed quiver.core is exactly the case that
// contract exists for; it is quiver.core's own deliberate self-restart that is
// the one exception.
func recoveryCommandFor(
	ns domain.Namespace,
	rt domainRuntime.ArrowRuntime,
) asynxModels.Command[domainRuntime.ArrowRuntime] {
	self, _ := metadata.GetSelfNamespaces()
	if strings.HasPrefix(ns.String(), string(self)+"@") {
		return runtimecmds.RecordSelfRestored{Namespace: ns, Execution: rt.Execution}
	}
	return runtimecmds.RecordDetached{Namespace: ns}
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
