package graphinternal

import (
	"context"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph/internal/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// Register subscribes projection handlers to arrow.added.* and arrow.updated.*
// events, writing dep edges into edgeStore. OnForget removes edges for forgotten arrows.
func Register(
	axArrow asynx.Asynx[domain.Arrow],
	edgeStore store.DepEdgeStore,
) error {
	p := &projections{store: edgeStore}

	if _, err := axArrow.Subscribe(asynx.Topic("arrow.added.*"), p.handleUpsert); err != nil {
		return err
	}

	if _, err := axArrow.Subscribe(asynx.Topic("arrow.upgraded.*"), p.handleUpsert); err != nil {
		return err
	}

	if _, err := axArrow.Subscribe(asynx.Topic("arrow.updated.*"), p.handleUpsert); err != nil {
		return err
	}

	if _, err := axArrow.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		p.handleRemove(ctx, evt)
	}); err != nil {
		return err
	}

	return nil
}

type projections struct {
	store store.DepEdgeStore
}

func (p *projections) handleUpsert(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate
	edges := CollectEdges(&arrow)

	ref := arrow.Namespace.Ref()
	if ref == "" {
		ref = domain.VersionLatestRef
	}

	fromNs := arrow.Namespace.BareNamespace().String()
	rows := toStoreRows(fromNs, ref, edges)

	if err := p.store.Save(ctx, fromNs, ref, rows); err != nil {
		slog.WarnContext(
			ctx, "graph: save edges failed",
			"namespace", arrow.Namespace,
			"err", err,
		)
	}
}

func (p *projections) handleRemove(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate
	ref := arrow.Namespace.Ref()
	if ref == "" {
		ref = domain.VersionLatestRef
	}

	fromNs := arrow.Namespace.BareNamespace().String()
	if err := p.store.DeleteFrom(ctx, fromNs, ref); err != nil {
		slog.WarnContext(
			ctx, "graph: delete edges failed",
			"namespace", arrow.Namespace,
			"err", err,
		)
	}
}

func toStoreRows(
	fromNs string,
	fromVersion string,
	edges []domain.DependencyEdge,
) []store.DepEdgeRow {
	rows := make([]store.DepEdgeRow, 0, len(edges))
	for _, e := range edges {
		rows = append(rows, store.DepEdgeRow{
			FromNamespace: fromNs,
			FromVersion:   fromVersion,
			ToNamespace:   e.Namespace.BareNamespace().String(),
			ToVersion:     e.Namespace.Ref(),
			Constraint:    e.Constraint,
			DepType:       string(e.Type),
		})
	}
	return rows
}
