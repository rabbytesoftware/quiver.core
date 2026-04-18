package graph

import (
	"context"
	"fmt"
	"log/slog"
	"path"

	"github.com/char2cs/asynx"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/graph/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// Graph manages dependency edges between arrow versions.
type Graph interface {
	SaveEdges(
		ctx context.Context,
		fromNs domain.Namespace,
		fromVersion string,
		edges []domain.DependencyEdge,
	) error
	DeleteEdges(
		ctx context.Context,
		fromNs domain.Namespace,
		fromVersion string,
	) error
	Dependents(
		ctx context.Context,
		toNs domain.Namespace,
		toVersion string,
	) ([]domain.DependencyEdge, error)
	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)
	CanUpgrade(
		ctx context.Context,
		toNs domain.Namespace,
		fromVersion string,
		toVersion string,
	) ([]domain.DependencyEdge, error)
}

type graphService struct {
	store store.DepEdgeStore
}

// New creates a graphService, registers Arrow aggregate projections, and returns a Graph.
func New(
	axArrow asynx.Asynx[domain.Arrow],
	st store.DepEdgeStore,
) (Graph, error) {
	g := &graphService{store: st}

	if err := g.registerProjections(axArrow); err != nil {
		return nil, fmt.Errorf("graph: register projections: %w", err)
	}

	return g, nil
}

func (g *graphService) SaveEdges(
	ctx context.Context,
	fromNs domain.Namespace,
	fromVersion string,
	edges []domain.DependencyEdge,
) error {
	rows := make([]store.DepEdgeRow, 0, len(edges))
	for _, e := range edges {
		rows = append(rows, store.DepEdgeRow{
			FromNamespace: fromNs.BareNamespace().String(),
			FromVersion:   fromVersion,
			ToNamespace:   e.Namespace.BareNamespace().String(),
			ToVersion:     e.Namespace.Ref(),
			Constraint:    e.Constraint,
			DepType:       string(e.Type),
		})
	}
	return g.store.Save(ctx, fromNs.BareNamespace().String(), fromVersion, rows)
}

func (g *graphService) DeleteEdges(
	ctx context.Context,
	fromNs domain.Namespace,
	fromVersion string,
) error {
	return g.store.DeleteFrom(ctx, fromNs.BareNamespace().String(), fromVersion)
}

func (g *graphService) Dependents(
	ctx context.Context,
	toNs domain.Namespace,
	toVersion string,
) ([]domain.DependencyEdge, error) {
	rows, err := g.store.ByDependency(ctx, toNs.BareNamespace().String(), toVersion)
	if err != nil {
		return nil, err
	}

	edges := make([]domain.DependencyEdge, 0, len(rows))
	for _, row := range rows {
		edges = append(edges, domain.DependencyEdge{
			Namespace:  domain.Namespace(row.FromNamespace).WithRef(row.FromVersion),
			Constraint: row.Constraint,
			Type:       domain.DepType(row.DepType),
		})
	}

	return edges, nil
}

func (g *graphService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return g.store.HasAnyDependents(
		ctx,
		ns.BareNamespace().String(),
		excludeNs.BareNamespace().String(),
	)
}

func (g *graphService) CanUpgrade(
	ctx context.Context,
	toNs domain.Namespace,
	fromVersion string,
	toVersion string,
) ([]domain.DependencyEdge, error) {
	rows, err := g.store.ByDependency(ctx, toNs.BareNamespace().String(), fromVersion)
	if err != nil {
		return nil, err
	}

	var compatible []domain.DependencyEdge
	for _, row := range rows {
		matched, err := path.Match(row.Constraint, toVersion)
		if err != nil {
			slog.WarnContext(ctx, "graph: CanUpgrade: malformed constraint",
				"constraint", row.Constraint,
				"err", err,
			)
			continue
		}
		if !matched {
			continue
		}
		compatible = append(compatible, domain.DependencyEdge{
			Namespace:  domain.Namespace(row.FromNamespace).WithRef(row.FromVersion),
			Constraint: row.Constraint,
			Type:       domain.DepType(row.DepType),
		})
	}

	return compatible, nil
}
