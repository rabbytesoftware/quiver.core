package graph

import (
	"context"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/char2cs/asynx"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (g *graphService) registerProjections(axArrow asynx.Asynx[domain.Arrow]) error {
	if _, err := axArrow.Subscribe("arrow.added", g.handleUpsert); err != nil {
		return err
	}

	if _, err := axArrow.Subscribe("arrow.updated", g.handleUpsert); err != nil {
		return err
	}

	if _, err := axArrow.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		g.handleRemove(ctx, evt)
	}); err != nil {
		return err
	}

	return nil
}

func (g *graphService) handleUpsert(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate
	for version, av := range arrow.Versions {
		edges := collectEdges(av)
		if err := g.SaveEdges(ctx, arrow.Namespace, version, edges); err != nil {
			slog.WarnContext(ctx, "graph: save edges failed",
				"namespace", arrow.Namespace,
				"version", version,
				"err", err,
			)
		}
	}
}

func (g *graphService) handleRemove(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate
	for version := range arrow.Versions {
		if err := g.DeleteEdges(ctx, arrow.Namespace, version); err != nil {
			slog.WarnContext(ctx, "graph: delete edges failed",
				"namespace", arrow.Namespace,
				"version", version,
				"err", err,
			)
		}
	}
}

// collectEdges aggregates DependencyEdges from all OS targets, deduplicated by namespace string.
func collectEdges(av domain.ArrowVersion) []domain.DependencyEdge {
	seen := make(map[string]struct{})
	var edges []domain.DependencyEdge

	for _, target := range av.Targets {
		for _, e := range target.Tools {
			key := e.Namespace.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			edges = append(edges, e)
		}
		for _, e := range target.Services {
			key := e.Namespace.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			edges = append(edges, e)
		}
	}

	return edges
}
