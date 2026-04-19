package deps_test

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	rootNs = domain.Namespace("github.com/org/root")
	toolNs = domain.Namespace("github.com/org/tool")
	svcNs  = domain.Namespace("github.com/org/svc")
	depANs = domain.Namespace("github.com/org/depA")
	depBNs = domain.Namespace("github.com/org/depB")
)

func makeResolveFunc(manifests map[domain.Namespace]*domain.ArrowManifest) func(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.ArrowManifest, error) {
	return func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.ArrowManifest, error) {
		if m, ok := manifests[ns.BareNamespace()]; ok {
			return m, nil
		}
		return &domain.ArrowManifest{}, nil
	}
}

func newService(
	manifests map[domain.Namespace]*domain.ArrowManifest,
) deps.Deps {
	return deps.NewTestable(
		deptree.New(),
		makeResolveFunc(manifests),
		nil,
		nil,
		nil,
		nil,
	)
}

func TestResolve_NoDeps_EmptyPlan(t *testing.T) {
	manifests := map[domain.Namespace]*domain.ArrowManifest{
		rootNs: {},
	}

	svc := newService(manifests)
	plan, err := svc.Resolve(context.Background(), rootNs)

	require.NoError(t, err)
	assert.Empty(t, plan)
}

func TestResolve_ToolDep(t *testing.T) {
	manifests := map[domain.Namespace]*domain.ArrowManifest{
		rootNs: {
			Targets: map[domain.OS]domain.Target{
				domain.OSDarwinAMD64: {
					Tools: []domain.DependencyEdge{
						{Namespace: toolNs, Type: domain.ToolDep},
					},
				},
			},
		},
		toolNs: {},
	}

	svc := newService(manifests)
	plan, err := svc.Resolve(context.Background(), rootNs)

	require.NoError(t, err)
	require.Len(t, plan, 1)
	assert.Equal(t, toolNs, plan[0].Namespace)
	assert.Equal(t, domain.ToolDep, plan[0].Type)
}

func TestResolve_ServiceDep(t *testing.T) {
	manifests := map[domain.Namespace]*domain.ArrowManifest{
		rootNs: {
			Targets: map[domain.OS]domain.Target{
				domain.OSDarwinAMD64: {
					Services: []domain.DependencyEdge{
						{Namespace: svcNs, Type: domain.ServiceDep},
					},
				},
			},
		},
		svcNs: {},
	}

	svc := newService(manifests)
	plan, err := svc.Resolve(context.Background(), rootNs)

	require.NoError(t, err)
	require.Len(t, plan, 1)
	assert.Equal(t, svcNs, plan[0].Namespace)
	assert.Equal(t, domain.ServiceDep, plan[0].Type)
}

func TestDiffDeps_AddsAndRemoves(t *testing.T) {
	oldManifest := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinAMD64: {
				Tools: []domain.DependencyEdge{
					{Namespace: depANs, Type: domain.ToolDep},
				},
			},
		},
	}
	newManifest := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinAMD64: {
				Tools: []domain.DependencyEdge{
					{Namespace: depANs, Type: domain.ToolDep},
					{Namespace: depBNs, Type: domain.ToolDep},
				},
			},
		},
	}

	svc := newService(nil)

	diff := svc.DiffDeps(oldManifest, newManifest)

	assert.Len(t, diff.Added, 1)
	assert.Equal(t, depBNs, diff.Added[0].Namespace)
	assert.Empty(t, diff.Removed)
}

func TestDiffDeps_Removed(t *testing.T) {
	oldManifest := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinAMD64: {
				Tools: []domain.DependencyEdge{
					{Namespace: depANs, Type: domain.ToolDep},
					{Namespace: depBNs, Type: domain.ToolDep},
				},
			},
		},
	}
	newManifest := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinAMD64: {
				Tools: []domain.DependencyEdge{
					{Namespace: depANs, Type: domain.ToolDep},
				},
			},
		},
	}

	svc := newService(nil)

	diff := svc.DiffDeps(oldManifest, newManifest)

	assert.Empty(t, diff.Added)
	assert.Len(t, diff.Removed, 1)
	assert.Equal(t, depBNs, diff.Removed[0].Namespace)
}
