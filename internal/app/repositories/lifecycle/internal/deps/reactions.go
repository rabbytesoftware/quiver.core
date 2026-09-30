package deps

import (
	"context"
	"log/slog"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func (d *deps) OnRuntimeEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	if rt.LastReturn == nil {
		return
	}

	switch rt.LastReturn.Method {
	case domain.MethodStop:
		d.onStopEnded(ctx, rt)
	case domain.MethodUninstall:
		d.onUninstallEnded(ctx, rt)
	case domain.MethodUpdate:
		d.updateEnded(ctx, rt)
	}
}

func (d *deps) onStopEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	plan, err := d.graph.Resolve(ctx, rt.Ref)
	if err != nil {
		return
	}

	for _, entry := range plan {
		if entry.Type != domain.ServiceDep {
			continue
		}
		depNs := entry.Namespace
		state, stateErr := d.runtime.GetState(ctx, depNs)
		if stateErr != nil {
			continue
		}
		if state != domain.ArrowStateRunning && state != domain.ArrowStateStopping {
			continue
		}

		parents, parentsErr := d.graph.GetDependents(ctx, depNs)
		if parentsErr != nil {
			slog.ErrorContext(ctx, "onStopEnded: GetDependents failed", "ns", depNs, "err", parentsErr)
			continue
		}

		filteredParents := make([]domain.Namespace, 0, len(parents))
		for _, p := range parents {
			if p != rt.Ref {
				filteredParents = append(filteredParents, p)
			}
		}

		if countRunning(ctx, filteredParents, d.runtime.GetState) == 0 {
			_ = d.runtime.BeginStop(ctx, depNs)
		}
	}

	// After cascading stops, check if the arrow that just stopped is itself
	// an orphaned non-user-installed dep that should be auto-uninstalled.
	d.maybeAutoUninstallStopped(ctx, rt.Ref)
}

func (d *deps) maybeAutoUninstallStopped(ctx context.Context, ns domain.Namespace) {
	arrow, err := d.arrow.Get(ctx, ns)
	if err != nil || arrow == nil {
		return
	}
	if arrow.UserInstalled {
		return
	}

	parents, err := d.graph.GetDependents(ctx, ns)
	if err != nil {
		return
	}

	if countRunning(ctx, parents, d.runtime.GetState) > 0 {
		return
	}

	_ = d.runtime.BeginUninstall(ctx, ns, nil)
}

func (d *deps) onUninstallEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) { //nolint:gocyclo
	plan, err := d.graph.Resolve(ctx, rt.Ref)
	if err != nil {
		return
	}

	for _, entry := range plan {
		depNs, catErr := d.arrow.ResolveCatalogued(ctx, entry.Namespace)
		if catErr != nil {
			continue
		}
		state, stateErr := d.runtime.GetState(ctx, depNs)
		if stateErr != nil {
			continue
		}
		if state == domain.ArrowStateAbsent || state == domain.ArrowStateRemoved || state == "" {
			continue
		}

		parents, parentsErr := d.graph.GetDependents(ctx, depNs)
		if parentsErr != nil {
			slog.ErrorContext(ctx, "onUninstallEnded: GetDependents failed", "ns", depNs, "err", parentsErr)
			continue
		}

		filteredParents := make([]domain.Namespace, 0, len(parents))
		for _, p := range parents {
			if p != rt.Ref {
				filteredParents = append(filteredParents, p)
			}
		}

		if countRunning(ctx, filteredParents, d.runtime.GetState) > 0 {
			continue
		}

		arrow, getErr := d.arrow.Get(ctx, depNs)
		if getErr != nil || arrow == nil || arrow.UserInstalled {
			continue
		}

		switch state {
		case domain.ArrowStateRunning, domain.ArrowStateStopping:
			_ = d.runtime.BeginStop(ctx, depNs)
		case domain.ArrowStateReady, domain.ArrowStateOutdated:
			_ = d.runtime.BeginUninstall(ctx, depNs, nil)
		case domain.ArrowStateAbsent,
			domain.ArrowStateInstalling,
			domain.ArrowStateUpdating,
			domain.ArrowStateDraining,
			domain.ArrowStateDetached,
			domain.ArrowStateUninstalling,
			domain.ArrowStateRemoved:
		}
	}
}

func countRunning(
	ctx context.Context,
	nss []domain.Namespace,
	getState func(
		context.Context,
		domain.Namespace,
	) (domain.ArrowState, error),
) int {
	n := 0
	for _, ns := range nss {
		s, _ := getState(ctx, ns)
		if s != domain.ArrowStateAbsent && s != domain.ArrowStateRemoved && s != "" {
			n++
		}
	}
	return n
}
