package deps

import (
	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func collectEdges(
	m *domain.Arrow,
) []domain.DependencyEdge {
	if m == nil {
		return nil
	}

	seen := make(map[domain.Namespace]struct{})
	var edges []domain.DependencyEdge

	for _, target := range m.Targets {
		for _, e := range target.Tools {
			bare := e.Namespace.BareNamespace()
			if _, ok := seen[bare]; ok {
				continue
			}
			seen[bare] = struct{}{}
			edges = append(edges, e)
		}
		for _, e := range target.Services {
			bare := e.Namespace.BareNamespace()
			if _, ok := seen[bare]; ok {
				continue
			}
			seen[bare] = struct{}{}
			edges = append(edges, e)
		}
	}

	return edges
}

func dedupNamespaces(
	ns []domain.Namespace,
) []domain.Namespace {
	seen := make(map[domain.Namespace]struct{}, len(ns))
	var result []domain.Namespace
	for _, n := range ns {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		result = append(result, n)
	}
	return result
}

// collectEdgesForOS returns dependency edges for the given OS target only.
func collectEdgesForOS(
	arrow *domain.Arrow,
	os domain.OS,
) []domain.DependencyEdge {
	if arrow == nil {
		return nil
	}
	target, ok := arrow.Targets[os]
	if !ok {
		return nil
	}

	seen := make(map[domain.Namespace]struct{})
	var edges []domain.DependencyEdge

	for _, e := range target.Tools {
		bare := e.Namespace.BareNamespace()
		if _, ok := seen[bare]; ok {
			continue
		}
		seen[bare] = struct{}{}
		edges = append(edges, e)
	}
	for _, e := range target.Services {
		bare := e.Namespace.BareNamespace()
		if _, ok := seen[bare]; ok {
			continue
		}
		seen[bare] = struct{}{}
		edges = append(edges, e)
	}
	return edges
}

func collectEdgesFromManifest(
	av domain.Arrow,
) []domain.DependencyEdge {
	return collectEdges(&av)
}

func edgesToRows(
	fromNs string,
	fromVersion string,
	edges []domain.DependencyEdge,
) []depsstore.DepEdgeRow {
	rows := make([]depsstore.DepEdgeRow, 0, len(edges))
	for _, e := range edges {
		rows = append(rows, depsstore.DepEdgeRow{
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
