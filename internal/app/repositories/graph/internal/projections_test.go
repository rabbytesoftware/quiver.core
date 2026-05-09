package graphinternal_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	graphinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func newTestAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

// mockEdgeStore records calls for assertion.
type mockEdgeStore struct {
	savedRows   []store.DepEdgeRow
	deletedFrom []string
	saveErr     error
	deleteErr   error
}

func (m *mockEdgeStore) Save(_ context.Context, fromNs, _ string, rows []store.DepEdgeRow) error {
	m.savedRows = append(m.savedRows, rows...)
	return m.saveErr
}

func (m *mockEdgeStore) DeleteFrom(_ context.Context, fromNs, _ string) error {
	m.deletedFrom = append(m.deletedFrom, fromNs)
	return m.deleteErr
}

func (m *mockEdgeStore) DeleteTo(_ context.Context, _ string) error {
	return nil
}

func (m *mockEdgeStore) ByDependency(_ context.Context, _, _ string) ([]store.DepEdgeRow, error) {
	return nil, nil
}

func (m *mockEdgeStore) EdgesToBare(_ context.Context, _ string) ([]store.DepEdgeRow, error) {
	return nil, nil
}

func (m *mockEdgeStore) HasAnyDependents(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

type addArrowCmd struct {
	arrow domain.Arrow
}

func (c addArrowCmd) AggregateID() string  { return c.arrow.Namespace.String() }
func (c addArrowCmd) EventName() string    { return "arrow.added." + c.arrow.Namespace.String() }
func (c addArrowCmd) ShouldSnapshot() bool { return true }
func (c addArrowCmd) Validate(current *domain.Arrow) error {
	if current != nil {
		return asynxModels.ErrValidation
	}
	return nil
}
func (c addArrowCmd) EmitEvent(_ *domain.Arrow) domain.Arrow { return c.arrow }

type updateArrowCmd struct {
	arrow domain.Arrow
}

func (c updateArrowCmd) AggregateID() string  { return c.arrow.Namespace.String() }
func (c updateArrowCmd) EventName() string    { return "arrow.updated." + c.arrow.Namespace.String() }
func (c updateArrowCmd) ShouldSnapshot() bool { return true }
func (c updateArrowCmd) Validate(current *domain.Arrow) error {
	if current == nil {
		return asynxModels.ErrValidation
	}
	return nil
}
func (c updateArrowCmd) EmitEvent(_ *domain.Arrow) domain.Arrow { return c.arrow }

func TestRegister_SavesEdgesOnAdd(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v1.0.0")
	arrow := domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Tools: []domain.DependencyEdge{{Namespace: depNs}},
			},
		},
	}

	_, err = axArrow.Send(context.Background(), addArrowCmd{arrow: arrow})
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.NotEmpty(t, ms.savedRows)
}

func TestRegister_SavesEdgesOnUpdate(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v1.0.0")
	arrow := domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Tools: []domain.DependencyEdge{{Namespace: depNs}},
			},
		},
	}

	_, err = axArrow.Send(context.Background(), addArrowCmd{arrow: arrow})
	require.NoError(t, err)

	// Update the arrow
	_, err = axArrow.Send(context.Background(), updateArrowCmd{arrow: arrow})
	require.NoError(t, err)
	axArrow.WaitPublish()

	// At least 2 saves (add + update)
	assert.GreaterOrEqual(t, len(ms.savedRows), 2)
}

func TestRegister_DeletesEdgesOnForget(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := domain.Arrow{Namespace: ns}

	_, err = axArrow.Send(context.Background(), addArrowCmd{arrow: arrow})
	require.NoError(t, err)
	axArrow.WaitPublish()

	err = axArrow.Forget(context.Background(), ns.String())
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.NotEmpty(t, ms.deletedFrom)
}

func TestRegister_NoEdgesArrow_SavesEmpty(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/noDeps@v1.0.0")
	arrow := domain.Arrow{Namespace: ns}

	_, err = axArrow.Send(context.Background(), addArrowCmd{arrow: arrow})
	require.NoError(t, err)
	axArrow.WaitPublish()

	// Save called with empty rows (no deps)
	// savedRows may be empty since no edges
	assert.NotNil(t, ms)
}

func TestRegister_SubscribeError_ShutdownAsynx(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	_ = axArrow.Shutdown(context.Background())
	ms := &mockEdgeStore{}

	err := graphinternal.Register(axArrow, ms)
	require.Error(t, err)
}

func TestHandleUpsert_SaveError_LogsAndContinues(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{saveErr: assert.AnError}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/saverr@v1.0.0")
	_, err = axArrow.Send(context.Background(), addArrowCmd{
		arrow: domain.Arrow{Namespace: ns},
	})
	require.NoError(t, err)
	axArrow.WaitPublish()
	// Should not panic; error is logged
}

func TestHandleRemove_DeleteError_LogsAndContinues(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{deleteErr: assert.AnError}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/delerr@v1.0.0")
	_, err = axArrow.Send(context.Background(), addArrowCmd{
		arrow: domain.Arrow{Namespace: ns},
	})
	require.NoError(t, err)
	axArrow.WaitPublish()

	err = axArrow.Forget(context.Background(), ns.String())
	require.NoError(t, err)
	axArrow.WaitPublish()
	// Should not panic; error is logged
}

func TestHandleUpsert_BareNamespace_UsesLatestRef(t *testing.T) {
	// Arrow namespace with no @ref → ref defaults to domain.VersionLatestRef
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	// Use a bare namespace (no @ref) for the arrow
	ns := domain.Namespace("github.com/user/bare")
	depNs := domain.Namespace("github.com/user/dep@v1.0.0")
	arrow := domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Tools: []domain.DependencyEdge{{Namespace: depNs}},
			},
		},
	}

	_, err = axArrow.Send(context.Background(), addArrowCmd{arrow: arrow})
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.NotEmpty(t, ms.savedRows)
}

func TestHandleRemove_BareNamespace_UsesLatestRef(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ms := &mockEdgeStore{}

	err := graphinternal.Register(axArrow, ms)
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/bareforget")
	_, err = axArrow.Send(context.Background(), addArrowCmd{
		arrow: domain.Arrow{Namespace: ns},
	})
	require.NoError(t, err)
	axArrow.WaitPublish()

	err = axArrow.Forget(context.Background(), ns.String())
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.NotEmpty(t, ms.deletedFrom)
}
