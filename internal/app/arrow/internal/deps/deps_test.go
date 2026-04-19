package deps_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	dbsqlite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/mocks"
	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
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

// --- helpers ---

func makeResolveFunc(
	manifests map[domain.Namespace]*domain.ArrowManifest,
) func(ctx context.Context, ns domain.Namespace) (*domain.ArrowManifest, error) {
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

func newExecutorService(
	install deps.InstallSyncFunc,
	startFn deps.StartFunc,
	uninstall deps.UninstallSyncFunc,
) deps.Deps {
	return deps.NewTestable(
		deptree.New(),
		makeResolveFunc(nil),
		nil,
		install,
		startFn,
		uninstall,
	)
}

func newCleanupService(
	st *mocks.StubStore,
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

func newAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	return ax
}

func newDepEdgeStore(t *testing.T) depsstore.DepEdgeStore {
	t.Helper()
	db, err := dbsqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := depsstore.NewDepEdgeStore(db)
	require.NoError(t, err)
	return st
}

// --- Resolve ---

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

func TestResolve_DepTreeError(t *testing.T) {
	wantErr := errors.New("deptree exploded")

	svc := deps.NewTestable(
		&failingDepTree{err: wantErr},
		makeResolveFunc(nil),
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.Resolve(context.Background(), rootNs)

	require.ErrorIs(t, err, wantErr)
}

// --- Unplan ---

func TestUnplan_DepTreeError(t *testing.T) {
	wantErr := errors.New("deptree exploded in unplan")

	svc := deps.NewTestable(
		&failingDepTree{err: wantErr},
		makeResolveFunc(nil),
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.Unplan(context.Background(), rootNs)

	require.ErrorIs(t, err, wantErr)
}

func TestUnplan_ReversesOrder(t *testing.T) {
	manifests := map[domain.Namespace]*domain.ArrowManifest{
		rootNs: {
			Targets: map[domain.OS]domain.Target{
				domain.OSDarwinAMD64: {
					Tools: []domain.DependencyEdge{
						{Namespace: toolNs, Type: domain.ToolDep},
						{Namespace: svcNs, Type: domain.ToolDep},
					},
				},
			},
		},
		toolNs: {},
		svcNs:  {},
	}

	svc := newService(manifests)
	plan, err := svc.Resolve(context.Background(), rootNs)
	require.NoError(t, err)
	require.NotEmpty(t, plan)

	unplan, err := svc.Unplan(context.Background(), rootNs)
	require.NoError(t, err)

	require.Len(t, unplan, len(plan))
	for i, entry := range plan {
		assert.Equal(t, entry.Namespace, unplan[len(plan)-1-i].Namespace)
	}
}

// --- DiffDeps ---

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

// --- Execute ---

func TestExecute_EmptyPlan_NoOp(t *testing.T) {
	svc := newExecutorService(nil, nil, nil)

	err := svc.Execute(context.Background(), deps.Plan{})

	require.NoError(t, err)
}

func TestExecute_InstallsToolDepsInOrder(t *testing.T) {
	var installed []domain.Namespace
	var started []domain.Namespace

	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		installed = append(installed, ns)
		return nil
	}

	startFn := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		started = append(started, ns)
		return nil
	}

	svc := newExecutorService(install, startFn, nil)

	plan := deps.Plan{
		{Namespace: toolNs, Type: domain.ToolDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{toolNs}, installed)
	assert.Empty(t, started)
}

func TestExecute_StartsServiceDepsAfterInstall(t *testing.T) {
	var installed []domain.Namespace
	var started []domain.Namespace

	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		installed = append(installed, ns)
		return nil
	}

	startFn := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		started = append(started, ns)
		return nil
	}

	svc := newExecutorService(install, startFn, nil)

	plan := deps.Plan{
		{Namespace: svcNs, Type: domain.ServiceDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{svcNs}, installed)
	assert.Equal(t, []domain.Namespace{svcNs}, started)
}

func TestExecute_RollsBackOnFailure(t *testing.T) {
	installErr := errors.New("install failed")
	var uninstalled []domain.Namespace

	callCount := 0
	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		callCount++
		if callCount == 2 {
			return installErr
		}
		return nil
	}

	uninstall := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		uninstalled = append(uninstalled, ns)
		return nil
	}

	svc := newExecutorService(install, nil, uninstall)

	plan := deps.Plan{
		{Namespace: toolNs, Type: domain.ToolDep},
		{Namespace: svcNs, Type: domain.ServiceDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.ErrorIs(t, err, installErr)
	assert.Equal(t, []domain.Namespace{toolNs}, uninstalled)
}

func TestExecute_RollbackIgnoresUninstallErrors(t *testing.T) {
	installErr := errors.New("install failed")

	callCount := 0
	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		callCount++
		if callCount == 2 {
			return installErr
		}
		return nil
	}

	uninstall := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		return errors.New("uninstall also failed")
	}

	svc := newExecutorService(install, nil, uninstall)

	plan := deps.Plan{
		{Namespace: toolNs, Type: domain.ToolDep},
		{Namespace: svcNs, Type: domain.ServiceDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.ErrorIs(t, err, installErr)
}

// --- HasDependents ---

func TestHasDependents_True(t *testing.T) {
	st := &mocks.StubStore{HasDependents: true}
	svc := newCleanupService(st, nil)

	result, err := svc.HasDependents(context.Background(), rootNs, rootNs)

	require.NoError(t, err)
	assert.True(t, result)
}

func TestHasDependents_False(t *testing.T) {
	st := &mocks.StubStore{HasDependents: false}
	svc := newCleanupService(st, nil)

	result, err := svc.HasDependents(context.Background(), rootNs, rootNs)

	require.NoError(t, err)
	assert.False(t, result)
}

func TestHasDependents_StoreError(t *testing.T) {
	someErr := errors.New("store error")
	st := &mocks.StubStore{DependentsErr: someErr}
	svc := newCleanupService(st, nil)

	result, err := svc.HasDependents(context.Background(), rootNs, rootNs)

	assert.ErrorIs(t, err, someErr)
	assert.False(t, result)
}

// --- Orphans ---

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

	st := &mocks.StubStore{HasDependents: false}
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

	st := &mocks.StubStore{HasDependents: true}
	svc := newCleanupService(st, manifests)

	orphans, err := svc.Orphans(context.Background(), rootNs)

	require.NoError(t, err)
	assert.Empty(t, orphans)
}

func TestOrphans_ResolveError_ReturnsError(t *testing.T) {
	wantErr := errors.New("deptree failed")

	svc := deps.NewTestable(
		&failingDepTree{err: wantErr},
		makeResolveFunc(nil),
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.Orphans(context.Background(), rootNs)

	require.ErrorIs(t, err, wantErr)
}

func TestOrphans_SkipsOnStoreError(t *testing.T) {
	storeErr := errors.New("db unreachable")
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

	st := &mocks.StubStore{DependentsErr: storeErr}
	svc := newCleanupService(st, manifests)

	orphans, err := svc.Orphans(context.Background(), rootNs)

	require.NoError(t, err)
	assert.Empty(t, orphans)
}

// --- New ---

func TestNew_RegistersProjections(t *testing.T) {
	ax := newAsynxArrow(t)
	st := newDepEdgeStore(t)

	d, err := deps.New(ax, nil, nil, st, nil, nil, nil)

	require.NoError(t, err)
	assert.NotNil(t, d)
}

func TestNew_ConstructsAndRegistersProjections(t *testing.T) {
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

	d, err := deps.New(axArrow, nil, nil, st, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, d)
}

func TestNew_SubscribeFailsOnFirstCall_ReturnsError(t *testing.T) {
	wantErr := assert.AnError
	ax := &failingAxArrow{subscribeCallN: 1, err: wantErr}
	st := newDepEdgeStore(t)

	d, err := deps.New(ax, nil, nil, st, nil, nil, nil)

	assert.Nil(t, d)
	require.ErrorIs(t, err, wantErr)
}

func TestNew_SubscribeFailsOnSecondCall_ReturnsError(t *testing.T) {
	wantErr := assert.AnError
	ax := &failingAxArrow{subscribeCallN: 2, err: wantErr}
	st := newDepEdgeStore(t)

	d, err := deps.New(ax, nil, nil, st, nil, nil, nil)

	assert.Nil(t, d)
	require.ErrorIs(t, err, wantErr)
}

func TestNew_OnForgetFailsToRegister_ReturnsError(t *testing.T) {
	wantErr := assert.AnError
	ax := &failingAxArrow{onForgetErr: wantErr}
	st := newDepEdgeStore(t)

	d, err := deps.New(ax, nil, nil, st, nil, nil, nil)

	assert.Nil(t, d)
	require.ErrorIs(t, err, wantErr)
}

// --- stubs ---

type failingDepTree struct {
	err error
}

func (f *failingDepTree) Resolve(
	ctx context.Context,
	root domain.Namespace,
	resolver deptree.ResolverFunc,
) ([]domain.Namespace, error) {
	return nil, f.err
}

type failingAxArrow struct {
	subscribeCallN int
	calls          int
	err            error
	onForgetErr    error
}

func (f *failingAxArrow) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	f.calls++
	if f.subscribeCallN > 0 && f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingAxArrow) Send(
	_ context.Context,
	_ asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAxArrow) SendWait(
	_ context.Context,
	_ asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAxArrow) Get(
	_ context.Context,
	_ string,
) (domain.Arrow, error) {
	return domain.Arrow{}, nil
}

func (f *failingAxArrow) Exists(
	_ context.Context,
	_ string,
) (bool, error) {
	return false, nil
}

func (f *failingAxArrow) Preload(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (f *failingAxArrow) Unsubscribe(_ string) error { return nil }

func (f *failingAxArrow) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domain.Arrow],
) error {
	return nil
}

func (f *failingAxArrow) Shutdown(_ context.Context) error { return nil }
func (f *failingAxArrow) WaitPublish()                     {}

func (f *failingAxArrow) Forget(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (f *failingAxArrow) OnForget(
	_ asynxModels.ForgetHandler[domain.Arrow],
) (string, error) {
	if f.onForgetErr != nil {
		return "", f.onForgetErr
	}
	return "forget-sub-id", nil
}
