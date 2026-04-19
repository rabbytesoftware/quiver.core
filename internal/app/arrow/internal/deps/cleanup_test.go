package deps_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	store "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubStore struct {
	hasDependents bool
	dependentsErr error
}

func (s *stubStore) HasAnyDependents(
	_ context.Context,
	_, _ string,
) (bool, error) {
	return s.hasDependents, s.dependentsErr
}

func (s *stubStore) ByDependency(
	_ context.Context,
	_, _ string,
) ([]store.DepEdgeRow, error) {
	return nil, nil
}

func newCleanupService(
	st *stubStore,
	manifests map[domain.Namespace]*domain.ArrowManifest,
) deps.Deps {
	return deps.NewTestable(
		deptree.New(),
		makeResolveFunc(manifests),
		st,
		nil,
		nil,
		nil,
	)
}

func TestHasDependents_True(t *testing.T) {
	st := &stubStore{hasDependents: true}
	svc := newCleanupService(st, nil)

	result, err := svc.HasDependents(context.Background(), rootNs, rootNs)

	require.NoError(t, err)
	assert.True(t, result)
}

func TestHasDependents_False(t *testing.T) {
	st := &stubStore{hasDependents: false}
	svc := newCleanupService(st, nil)

	result, err := svc.HasDependents(context.Background(), rootNs, rootNs)

	require.NoError(t, err)
	assert.False(t, result)
}

func TestHasDependents_StoreError(t *testing.T) {
	someErr := errors.New("store error")
	st := &stubStore{dependentsErr: someErr}
	svc := newCleanupService(st, nil)

	result, err := svc.HasDependents(context.Background(), rootNs, rootNs)

	assert.ErrorIs(t, err, someErr)
	assert.False(t, result)
}

func TestOrphans_AllOrphans(t *testing.T) {
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

	st := &stubStore{hasDependents: false}
	svc := newCleanupService(st, manifests)

	orphans, err := svc.Orphans(context.Background(), rootNs)

	require.NoError(t, err)
	require.Len(t, orphans, 1)
	assert.Equal(t, toolNs, orphans[0])
}

func TestOrphans_NoneOrphans(t *testing.T) {
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

	st := &stubStore{hasDependents: true}
	svc := newCleanupService(st, manifests)

	orphans, err := svc.Orphans(context.Background(), rootNs)

	require.NoError(t, err)
	assert.Empty(t, orphans)
}
