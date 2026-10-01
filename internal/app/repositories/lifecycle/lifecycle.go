package lifecycle

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/bracket"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/commits"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/deps"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/settle"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// Lifecycle orchestrates what moves an arrow through its runtime: installs
// with their dependencies, uninstalls, executions, stops, and updates with
// their per-row bracket and the commit that settles them.
type Lifecycle interface {
	// Install installs ns after its dependencies and reports whether it began
	// an install: false when the row is already installed.
	Install(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) (bool, error)
	Uninstall(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	Execute(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	// Update updates ns to what is ahead of it and reports whether an update
	// started: false when nothing is newer, an idempotent no-op no runtime
	// event will follow.
	Update(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) (bool, error)
	Stop(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// Reset forgets the runtime aggregate, clearing a runtime stuck in a
	// transient state. The catalog entry (if any) is left intact so the arrow
	// can be re-installed.
	Reset(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// Recheck re-resolves what is ahead of ns. A row that is not installed is
	// advanced to it in the catalog; an installed one is left where it is,
	// since only Update runs the update steps that move it.
	Recheck(
		ctx context.Context,
		ns domain.Namespace,
	) (models.UpdateResult, error)
	// Settling reports whether an update of ns began and has not committed
	// yet. Its runtime may already read ready or outdated: the commit that
	// advances the row runs after the update's steps ended.
	Settling(ns domain.Namespace) bool
	// HoldBadge tells a version check whether to leave ns's badge alone:
	// while ns settles, the row still names the target about to be stamped.
	HoldBadge(ns domain.Namespace) bool
	// UpdateTarget reports the target the update of ns in flight began
	// toward: the release its run builds, whatever the row records since.
	UpdateTarget(ns domain.Namespace) (domain.Available, bool)
	// Drain waits for every update commit in flight and refuses new ones,
	// so a shutdown never closes the stores under a commit. When ctx ends
	// first it aborts them and reports why: those rows stay outdated and
	// their next update runs again.
	Drain(ctx context.Context) error
	// Start subscribes the reactions to a runtime's end: the dependency
	// cascades of a stop or an uninstall, and the settling of an update.
	Start() error
}

// Arrow is what the lifecycle needs from the arrow catalog.
type Arrow interface {
	bracket.Arrow
	settle.Arrow
	deps.Arrow
}

// Runtime is what the lifecycle needs from the runtime repository.
type Runtime interface {
	bracket.Runtime
	settle.Runtime
	deps.Runtime
	OnRuntimeEnded(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
}

// Graph is what the lifecycle needs from the dependency graph.
type Graph interface {
	bracket.Graph
	deps.Graph
}

type lifecycle struct {
	runtime  Runtime
	targets  bracket.Targets
	commits  commits.Commits
	settler  settle.Settler
	deps     deps.Deps
	updater  bracket.Updater
	advancer bracket.Advancer
}

func New(
	arrow Arrow,
	runtime Runtime,
	graph Graph,
) Lifecycle {
	targets := bracket.NewTargets()
	inFlight := commits.New()
	settler := settle.New(arrow, runtime, targets, inFlight)
	depsRunner := deps.New(arrow, runtime, graph, targets, settler.OnUpdateEnded)

	return &lifecycle{
		runtime:  runtime,
		targets:  targets,
		commits:  inFlight,
		settler:  settler,
		deps:     depsRunner,
		updater:  bracket.NewUpdater(arrow, runtime, targets, settler, depsRunner),
		advancer: bracket.NewAdvancer(arrow, runtime, graph, targets),
	}
}

func (l *lifecycle) Install(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	return l.deps.Install(ctx, ns, vars)
}

func (l *lifecycle) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) error {
	return l.deps.Uninstall(ctx, ns, vars)
}

func (l *lifecycle) Execute(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error {
	return l.updater.Execute(ctx, ns, method, vars)
}

func (l *lifecycle) Update(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	return l.updater.Update(ctx, ns, vars)
}

func (l *lifecycle) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return l.deps.Stop(ctx, ns)
}

func (l *lifecycle) Reset(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return l.updater.Reset(ctx, ns)
}

func (l *lifecycle) Recheck(
	ctx context.Context,
	ns domain.Namespace,
) (models.UpdateResult, error) {
	return l.advancer.Recheck(ctx, ns)
}

func (l *lifecycle) Settling(ns domain.Namespace) bool {
	return l.settler.Settling(ns)
}

func (l *lifecycle) HoldBadge(ns domain.Namespace) bool {
	return l.settler.HoldBadge(ns)
}

func (l *lifecycle) UpdateTarget(ns domain.Namespace) (domain.Available, bool) {
	return l.targets.Peek(ns)
}

func (l *lifecycle) Drain(ctx context.Context) error {
	return l.commits.Drain(ctx)
}

func (l *lifecycle) Start() error {
	if err := l.runtime.OnRuntimeEnded(l.deps.OnRuntimeEnded); err != nil {
		return fmt.Errorf("lifecycle: subscribe runtime ended: %w", err)
	}
	return nil
}
