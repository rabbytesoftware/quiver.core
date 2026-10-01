package deps

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/bracket"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const rollingRow = domain.Namespace("github.com/char2cs/crowbar@nightly-latest")

// harness runs deps with a real bracket and an update end that is recorded.
type harness struct {
	arrow        Arrow
	deps         *deps
	updatesEnded []domainRuntime.ArrowRuntime
}

func newUC(a *mocks.MockArrow, rt *mocks.MockRuntime, g *mocks.MockGraph) *harness {
	h := &harness{arrow: a}
	h.deps = New(a, rt, g, bracket.NewTargets(), func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		h.updatesEnded = append(h.updatesEnded, rt)
	}).(*deps)
	return h
}

func (h *harness) Install(ctx context.Context, ns domain.Namespace, vars map[string]string) (bool, error) {
	return h.deps.Install(ctx, ns, vars)
}

func (h *harness) Uninstall(ctx context.Context, ns domain.Namespace, vars map[string]string) error {
	return h.deps.Uninstall(ctx, ns, vars)
}

func (h *harness) Stop(ctx context.Context, ns domain.Namespace) error {
	return h.deps.Stop(ctx, ns)
}

func (h *harness) onRuntimeEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	h.deps.OnRuntimeEnded(ctx, rt)
}

func (h *harness) onUninstallEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	h.deps.onUninstallEnded(ctx, rt)
}

func (h *harness) installOneDep(ctx context.Context, ns domain.Namespace) error {
	return h.deps.installOneDep(ctx, ns)
}

func (h *harness) startServiceDep(ctx context.Context, ns domain.Namespace) error {
	return h.deps.startServiceDep(ctx, ns)
}

func (h *harness) syncDeps(ctx context.Context, ns domain.Namespace) error {
	return h.deps.syncDeps(ctx, ns)
}

func (h *harness) maybeAutoUninstallStopped(ctx context.Context, ns domain.Namespace) {
	h.deps.maybeAutoUninstallStopped(ctx, ns)
}
