package settle

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/bracket"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/commits"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/deps"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// harness wires the lifecycle's parts the way lifecycle.New does.
type harness struct {
	targets bracket.Targets
	commits commits.Commits
	settler *settler
	deps    deps.Deps
	updater bracket.Updater
	detach  func(fn func())
}

func newHarness(a *mocks.MockArrow, rt *mocks.MockRuntime, g *mocks.MockGraph, detach func(fn func())) *harness {
	h := &harness{
		targets: bracket.NewTargets(),
		commits: commits.New(),
		detach:  detach,
	}
	h.settler = New(a, rt, h.targets, h.commits, WithDetach(func(fn func()) { h.detach(fn) })).(*settler)
	h.deps = deps.New(a, rt, g, h.targets, h.settler.OnUpdateEnded)
	h.updater = bracket.NewUpdater(a, rt, h.targets, h.settler, h.deps)
	return h
}

// newUC runs the settling of an update inline, so a test observes it without
// waiting on a goroutine.
func newUC(a *mocks.MockArrow, rt *mocks.MockRuntime, g *mocks.MockGraph) *harness {
	return newHarness(a, rt, g, func(fn func()) { fn() })
}

// newDetachedUC settles an update off the delivering goroutine, as the
// lifecycle does.
func newDetachedUC(a *mocks.MockArrow, rt *mocks.MockRuntime, g *mocks.MockGraph) *harness {
	return newHarness(a, rt, g, func(fn func()) { go fn() })
}

func (h *harness) Execute(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
	return h.updater.Execute(ctx, ns, method, vars)
}

func (h *harness) Reset(ctx context.Context, ns domain.Namespace) error {
	return h.updater.Reset(ctx, ns)
}

func (h *harness) Settling(ns domain.Namespace) bool {
	return h.settler.Settling(ns)
}

func (h *harness) HoldBadge(ns domain.Namespace) bool {
	return h.settler.HoldBadge(ns)
}

func (h *harness) Drain(ctx context.Context) error {
	return h.commits.Drain(ctx)
}

func (h *harness) onRuntimeEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	h.deps.OnRuntimeEnded(ctx, rt)
}

func (h *harness) onUpdateEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	h.settler.OnUpdateEnded(ctx, rt)
}
