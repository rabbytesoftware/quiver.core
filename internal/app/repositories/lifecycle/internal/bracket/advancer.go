package bracket

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Advancer re-checks catalog rows and advances the ones nothing is installed
// from.
type Advancer interface {
	// Recheck re-resolves what is ahead of ns. A row that is not installed is
	// advanced to it in the catalog; an installed one is left where it is,
	// since only an update runs the steps that move it.
	Recheck(
		ctx context.Context,
		ns domain.Namespace,
	) (models.UpdateResult, error)
}

type advancer struct {
	arrow   Arrow
	runtime Runtime
	graph   Graph
	targets Targets
}

func NewAdvancer(
	arrow Arrow,
	runtime Runtime,
	graph Graph,
	targets Targets,
) Advancer {
	return &advancer{
		arrow:   arrow,
		runtime: runtime,
		graph:   graph,
		targets: targets,
	}
}

func (a *advancer) Recheck(
	ctx context.Context,
	ns domain.Namespace,
) (models.UpdateResult, error) {
	ns, err := a.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: %w", err)
	}

	current, err := a.arrow.Get(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: get current: %w", err)
	}
	available, err := a.arrow.CheckAvailable(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: %w", err)
	}
	if available == nil {
		return models.UpdateResult{}, nil
	}

	installed, err := a.installed(ctx, ns)
	if err != nil || installed {
		return models.UpdateResult{Available: available}, err
	}

	return a.advanceCatalogued(ctx, ns, current, *available)
}

func (a *advancer) installed(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	state, err := a.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("update: get state: %w", err)
	}
	return isInstalled(state), nil
}

// advanceCatalogued moves a row nothing is installed from straight to
// target: there are no update steps to run for it. It holds the row's
// bracket and reads the state again inside it, so an install that began in
// the meantime is never handed the manifest the row is leaving.
func (a *advancer) advanceCatalogued(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	target domain.Available,
) (models.UpdateResult, error) {
	closeBracket, err := a.targets.Open(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: %w", err)
	}
	defer closeBracket()
	installed, err := a.installed(ctx, ns)
	if err != nil || installed {
		return models.UpdateResult{Available: &target}, err
	}

	if err := a.arrow.Advance(ctx, ns, target); err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: %w", err)
	}
	advanced, err := a.arrow.Get(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: get advanced: %w", err)
	}

	diff := a.graph.DiffDeps(current, advanced)
	return models.UpdateResult{
		AddedDeps:           edgesToNs(diff.Added),
		RemovedFromManifest: edgesToNs(diff.Removed),
		ConstrainedDeps:     diff.Constrained,
	}, nil
}

func isInstalled(
	state domain.ArrowState,
) bool {
	return state != "" &&
		state != domain.ArrowStateAbsent &&
		state != domain.ArrowStateRemoved
}

func edgesToNs(edges []domain.DependencyEdge) []domain.Namespace {
	ns := make([]domain.Namespace, 0, len(edges))
	for _, e := range edges {
		ns = append(ns, e.Namespace)
	}
	return ns
}
