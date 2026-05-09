package graphinternal_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	graphinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func edge(ns string) domain.DependencyEdge {
	return domain.DependencyEdge{Namespace: domain.Namespace(ns)}
}

func TestCollectEdges_Nil(t *testing.T) {
	edges := graphinternal.CollectEdges(nil)
	assert.Nil(t, edges)
}

func TestCollectEdges_NoTargets(t *testing.T) {
	arrow := &domain.Arrow{}
	edges := graphinternal.CollectEdges(arrow)
	assert.Empty(t, edges)
}

func TestCollectEdges_ToolsAndServices(t *testing.T) {
	os := domain.OSDarwinARM64
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			os: {
				Tools:    []domain.DependencyEdge{edge("github.com/a/tool@v1")},
				Services: []domain.DependencyEdge{edge("github.com/b/svc@v1")},
			},
		},
	}
	edges := graphinternal.CollectEdges(arrow)
	assert.Len(t, edges, 2)
}

func TestCollectEdges_DeduplicatesByBare(t *testing.T) {
	os1 := domain.OSDarwinARM64
	os2 := domain.OSLinuxAMD64
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			os1: {Tools: []domain.DependencyEdge{edge("github.com/a/tool@v1")}},
			os2: {Tools: []domain.DependencyEdge{edge("github.com/a/tool@v2")}},
		},
	}
	edges := graphinternal.CollectEdges(arrow)
	// Both have same bare namespace → deduplicated
	assert.Len(t, edges, 1)
}

func TestCollectEdgesForOS_Nil(t *testing.T) {
	edges := graphinternal.CollectEdgesForOS(nil, domain.OSDarwinARM64)
	assert.Nil(t, edges)
}

func TestCollectEdgesForOS_OSNotFound(t *testing.T) {
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
	}
	edges := graphinternal.CollectEdgesForOS(arrow, domain.OSDarwinARM64)
	assert.Nil(t, edges)
}

func TestCollectEdgesForOS_ReturnsOnlyMatchingOS(t *testing.T) {
	os := domain.OSDarwinARM64
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			os: {
				Tools:    []domain.DependencyEdge{edge("github.com/a/tool@v1")},
				Services: []domain.DependencyEdge{edge("github.com/b/svc@v1")},
			},
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{edge("github.com/c/other@v1")},
			},
		},
	}
	edges := graphinternal.CollectEdgesForOS(arrow, os)
	assert.Len(t, edges, 2)
}

func TestCollectEdgesForOS_DeduplicatesWithinOS(t *testing.T) {
	os := domain.OSDarwinARM64
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			os: {
				Tools:    []domain.DependencyEdge{edge("github.com/a/tool@v1")},
				Services: []domain.DependencyEdge{edge("github.com/a/tool@v2")},
			},
		},
	}
	edges := graphinternal.CollectEdgesForOS(arrow, os)
	assert.Len(t, edges, 1)
}

func TestCollectEdges_DuplicateServices(t *testing.T) {
	// Two OS targets with the same Service bare namespace → only one edge collected.
	os1 := domain.OSDarwinARM64
	os2 := domain.OSLinuxAMD64
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			os1: {Services: []domain.DependencyEdge{edge("github.com/a/svc@v1")}},
			os2: {Services: []domain.DependencyEdge{edge("github.com/a/svc@v2")}},
		},
	}
	edges := graphinternal.CollectEdges(arrow)
	assert.Len(t, edges, 1)
}

func TestCollectEdgesForOS_DuplicateToolsWithinOS(t *testing.T) {
	// Same bare namespace appears twice in Tools list — should deduplicate.
	os := domain.OSDarwinARM64
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			os: {
				Tools: []domain.DependencyEdge{
					edge("github.com/a/tool@v1"),
					edge("github.com/a/tool@v2"), // same bare ns
				},
			},
		},
	}
	edges := graphinternal.CollectEdgesForOS(arrow, os)
	assert.Len(t, edges, 1)
}

func TestDedupNamespaces_Empty(t *testing.T) {
	result := graphinternal.DedupNamespaces(nil)
	assert.Nil(t, result)
}

func TestDedupNamespaces_NoDuplicates(t *testing.T) {
	ns := []domain.Namespace{"a", "b", "c"}
	result := graphinternal.DedupNamespaces(ns)
	assert.Equal(t, ns, result)
}

func TestDedupNamespaces_WithDuplicates(t *testing.T) {
	ns := []domain.Namespace{"a", "b", "a", "c", "b"}
	result := graphinternal.DedupNamespaces(ns)
	assert.Equal(t, []domain.Namespace{"a", "b", "c"}, result)
}
