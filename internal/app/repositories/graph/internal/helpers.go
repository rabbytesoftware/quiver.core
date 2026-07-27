package graphinternal

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// CollectEdgesForOS collects the dependency edges an arrow declares for one
// platform, deduplicating by bare namespace. Edges are per-platform because
// that is what the machine will actually install.
func CollectEdgesForOS(
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

// DedupNamespaces removes duplicate namespace entries preserving order.
func DedupNamespaces(
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
