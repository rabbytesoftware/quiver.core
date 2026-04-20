package deps

import (
	"testing"

	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectEdges_NilManifest(t *testing.T) {
	edges := collectEdges(nil)

	assert.Nil(t, edges)
}

func TestCollectEdges_DeduplicatesByBareNamespace(t *testing.T) {
	// Same bare namespace appears in both Tools and Services — should appear once.
	ns := domain.Namespace("github.com/user/tool@v1.0")
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinAMD64: {
				Tools: []domain.DependencyEdge{
					{Namespace: ns, Type: domain.ToolDep},
				},
				Services: []domain.DependencyEdge{
					{Namespace: ns, Type: domain.ServiceDep},
				},
			},
		},
	}

	edges := collectEdges(m)

	require.Len(t, edges, 1)
	assert.Equal(t, ns, edges[0].Namespace)
}

func TestCollectEdges_MultipleTargets(t *testing.T) {
	ns := domain.Namespace("github.com/user/tool@v1.0")
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinAMD64: {
				Tools: []domain.DependencyEdge{
					{Namespace: ns, Type: domain.ToolDep},
				},
			},
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					{Namespace: ns, Type: domain.ToolDep},
				},
			},
		},
	}

	edges := collectEdges(m)

	// Same bare namespace from both OS targets — deduplicated to one entry.
	require.Len(t, edges, 1)
	assert.Equal(t, ns, edges[0].Namespace)
}

func TestDedupNamespaces_Empty(t *testing.T) {
	result := dedupNamespaces(nil)

	assert.Nil(t, result)
}

func TestDedupNamespaces_NoDuplicates(t *testing.T) {
	input := []domain.Namespace{
		"github.com/a/b",
		"github.com/c/d",
	}

	result := dedupNamespaces(input)

	require.Len(t, result, 2)
}

func TestDedupNamespaces_WithDuplicates(t *testing.T) {
	input := []domain.Namespace{
		"github.com/a/b",
		"github.com/a/b",
		"github.com/c/d",
	}

	result := dedupNamespaces(input)

	require.Len(t, result, 2)
	assert.Equal(t, domain.Namespace("github.com/a/b"), result[0])
	assert.Equal(t, domain.Namespace("github.com/c/d"), result[1])
}

func TestEdgesToRows_MapsFields(t *testing.T) {
	fromNs := "github.com/user/from"
	fromVersion := "v1.0.0"
	edges := []domain.DependencyEdge{
		{
			Namespace:  domain.Namespace("github.com/user/tool@v2.0"),
			Constraint: "2.*",
			Type:       domain.ToolDep,
		},
	}

	rows := edgesToRows(fromNs, fromVersion, edges)

	require.Len(t, rows, 1)
	row := rows[0]
	assert.Equal(t, fromNs, row.FromNamespace)
	assert.Equal(t, fromVersion, row.FromVersion)
	assert.Equal(t, "github.com/user/tool", row.ToNamespace)
	assert.Equal(t, "v2.0", row.ToVersion)
	assert.Equal(t, "2.*", row.Constraint)
	assert.Equal(t, string(domain.ToolDep), row.DepType)
}

func TestEdgesToRows_EmptyEdges(t *testing.T) {
	rows := edgesToRows(
		"github.com/user/from",
		"v1.0.0",
		[]domain.DependencyEdge{},
	)

	assert.Empty(t, rows)
	assert.IsType(t, []depsstore.DepEdgeRow{}, rows)
}
