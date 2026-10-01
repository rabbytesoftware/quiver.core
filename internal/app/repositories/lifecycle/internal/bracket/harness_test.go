package bracket

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/commits"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/deps"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/settle"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// harness wires the lifecycle's parts the way lifecycle.New does.
type harness struct {
	targets *targets
	commits commits.Commits
	settler settle.Settler
	deps    deps.Deps
	updater Updater
	detach  func(fn func())
}

// newUC runs the settling of an update inline, so a test observes it without
// waiting on a goroutine.
func newUC(a *mocks.MockArrow, rt *mocks.MockRuntime, g *mocks.MockGraph) *harness {
	h := &harness{
		targets: NewTargets().(*targets),
		commits: commits.New(),
		detach:  func(fn func()) { fn() },
	}
	h.settler = settle.New(a, rt, h.targets, h.commits, settle.WithDetach(func(fn func()) { h.detach(fn) }))
	h.deps = deps.New(a, rt, g, h.targets, h.settler.OnUpdateEnded)
	h.updater = NewUpdater(a, rt, h.targets, h.settler, h.deps)
	return h
}

func (h *harness) Install(ctx context.Context, ns domain.Namespace, vars map[string]string) (bool, error) {
	return h.deps.Install(ctx, ns, vars)
}

func (h *harness) Execute(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
	return h.updater.Execute(ctx, ns, method, vars)
}

func (h *harness) Update(ctx context.Context, ns domain.Namespace, vars map[string]string) (bool, error) {
	return h.updater.Update(ctx, ns, vars)
}

func (h *harness) Reset(ctx context.Context, ns domain.Namespace) error {
	return h.updater.Reset(ctx, ns)
}

func (h *harness) Settling(ns domain.Namespace) bool {
	return h.settler.Settling(ns)
}

func (h *harness) onUpdateEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	h.settler.OnUpdateEnded(ctx, rt)
}
