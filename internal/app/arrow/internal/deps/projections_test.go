package deps_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	dbsqlite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingDepEdgeStore is a stub that returns an error on Save and DeleteFrom.
type failingDepEdgeStore struct {
	saveErr   error
	deleteErr error
}

func (f *failingDepEdgeStore) Save(
	_ context.Context,
	_ string,
	_ string,
	_ []depsstore.DepEdgeRow,
) error {
	return f.saveErr
}

func (f *failingDepEdgeStore) DeleteFrom(
	_ context.Context,
	_ string,
	_ string,
) error {
	return f.deleteErr
}

func (f *failingDepEdgeStore) ByDependency(
	_ context.Context,
	_ string,
	_ string,
) ([]depsstore.DepEdgeRow, error) {
	return nil, nil
}

func (f *failingDepEdgeStore) HasAnyDependents(
	_ context.Context,
	_ string,
	_ string,
) (bool, error) {
	return false, nil
}

type projFixture struct {
	d       deps.Deps
	store   depsstore.DepEdgeStore
	axArrow asynx.Asynx[domain.Arrow]
}

func newProjFixture(t *testing.T) *projFixture {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	db, err := dbsqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := depsstore.NewDepEdgeStore(db)
	require.NoError(t, err)

	d, err := deps.New(domain.OSDarwinAMD64, axArrow, nil, nil, nil, st, nil, nil, nil)
	require.NoError(t, err)

	return &projFixture{
		d:       d,
		store:   st,
		axArrow: axArrow,
	}
}

func TestProjections_HandleUpsert_SavesEdgesOnArrowAdded(t *testing.T) {
	ctx := context.Background()
	f := newProjFixture(t)

	toNs := domain.Namespace("github.com/user/tool@v1.0")
	manifest := domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					{
						Namespace:  toNs,
						Constraint: "1.*",
						Type:       domain.ToolDep,
					},
				},
			},
		},
	}

	_, err := f.axArrow.Send(ctx, commands.AddArrow{
		Namespace: domain.Namespace("github.com/user/from"),
		Targets:   manifest.Targets,
	})
	require.NoError(t, err)

	require.Eventually(
		t,
		func() bool {
			rows, _ := f.store.ByDependency(ctx, toNs.BareNamespace().String(), toNs.Ref())
			return len(rows) > 0
		},
		time.Second,
		10*time.Millisecond,
	)
}

func TestProjections_HandleRemove_DeletesEdgesOnForget(t *testing.T) {
	ctx := context.Background()
	f := newProjFixture(t)

	fromNs := domain.Namespace("github.com/user/from")
	toNs := domain.Namespace("github.com/user/tool@v1.0")
	manifest := domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					{
						Namespace:  toNs,
						Constraint: "1.*",
						Type:       domain.ToolDep,
					},
				},
			},
		},
	}

	_, err := f.axArrow.Send(ctx, commands.AddArrow{
		Namespace: fromNs,
		Targets:   manifest.Targets,
	})
	require.NoError(t, err)

	// Wait for the upsert projection to fire and save edges.
	require.Eventually(
		t,
		func() bool {
			rows, _ := f.store.ByDependency(ctx, toNs.BareNamespace().String(), toNs.Ref())
			return len(rows) > 0
		},
		time.Second,
		10*time.Millisecond,
	)

	err = f.axArrow.Forget(ctx, fromNs.String())
	require.NoError(t, err)

	// Wait for the remove projection to fire and delete edges.
	require.Eventually(
		t,
		func() bool {
			rows, _ := f.store.ByDependency(ctx, toNs.BareNamespace().String(), toNs.Ref())
			return len(rows) == 0
		},
		time.Second,
		10*time.Millisecond,
	)

	rows, err := f.store.ByDependency(ctx, toNs.BareNamespace().String(), toNs.Ref())
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func newProjFixtureWithStore(
	t *testing.T,
	st depsstore.DepEdgeStore,
) (deps.Deps, asynx.Asynx[domain.Arrow]) {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	d, err := deps.New(domain.OSDarwinAMD64, axArrow, nil, nil, nil, st, nil, nil, nil)
	require.NoError(t, err)

	return d, axArrow
}

func TestProjections_HandleUpsert_SaveError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	saveErr := errors.New("save failed")
	fs := &failingDepEdgeStore{saveErr: saveErr}

	_, axArrow := newProjFixtureWithStore(t, fs)

	manifest := domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					{
						Namespace:  domain.Namespace("github.com/user/tool@v1.0"),
						Constraint: "1.*",
						Type:       domain.ToolDep,
					},
				},
			},
		},
	}

	fromNs := domain.Namespace("github.com/user/from-fail")
	_, err := axArrow.Send(ctx, commands.AddArrow{
		Namespace: fromNs,
		Targets:   manifest.Targets,
	})
	require.NoError(t, err)

	// Wait for the projection to fire — it should log a warning but not panic.
	require.Eventually(
		t,
		func() bool {
			axArrow.WaitPublish()
			return true
		},
		time.Second,
		10*time.Millisecond,
	)
}

func TestProjections_HandleRemove_DeleteError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	deleteErr := errors.New("delete failed")
	fs := &failingDepEdgeStore{deleteErr: deleteErr}

	_, axArrow := newProjFixtureWithStore(t, fs)

	manifest := domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					{
						Namespace:  domain.Namespace("github.com/user/tool@v1.0"),
						Constraint: "1.*",
						Type:       domain.ToolDep,
					},
				},
			},
		},
	}

	fromNs := domain.Namespace("github.com/user/from-delete-fail")
	_, err := axArrow.Send(ctx, commands.AddArrow{
		Namespace: fromNs,
		Targets:   manifest.Targets,
	})
	require.NoError(t, err)
	axArrow.WaitPublish()

	err = axArrow.Forget(ctx, fromNs.String())
	require.NoError(t, err)

	// Wait for the remove projection to fire — it should log a warning but not panic.
	require.Eventually(
		t,
		func() bool {
			axArrow.WaitPublish()
			return true
		},
		time.Second,
		10*time.Millisecond,
	)
}
