package arrow

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	appDeps "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errAsynxRuntime is a minimal asynx.Asynx[domainRuntime.ArrowRuntime] that
// returns a fixed error from Get for testing error propagation.
type errAsynxRuntime struct {
	getErr error
}

func (e *errAsynxRuntime) Get(_ context.Context, _ string) (domainRuntime.ArrowRuntime, error) {
	return domainRuntime.ArrowRuntime{}, e.getErr
}

func (e *errAsynxRuntime) Send(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (e *errAsynxRuntime) SendWait(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (e *errAsynxRuntime) Shutdown(_ context.Context) error { return nil }

func (e *errAsynxRuntime) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (e *errAsynxRuntime) Preload(_ context.Context, _ string) error { return nil }

func (e *errAsynxRuntime) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	return "", nil
}

func (e *errAsynxRuntime) Unsubscribe(_ string) error { return nil }

func (e *errAsynxRuntime) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
) error {
	return nil
}

func (e *errAsynxRuntime) Forget(_ context.Context, _ string) error { return nil }
func (e *errAsynxRuntime) OnForget(_ asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error) {
	return "forget-sub-id", nil
}

func (e *errAsynxRuntime) WaitPublish() {}

// --- mock catalog ---

type mockCatalog struct {
	addErr      error
	updateErr   error
	removeErr   error
	listArrows  []domain.Arrow
	listErr     error
	getArrow    *domain.Arrow
	getErr      error
	isInstalled bool
}

func (m *mockCatalog) Add(_ context.Context, _ domain.Namespace) error {
	return m.addErr
}

func (m *mockCatalog) Update(_ context.Context, _ domain.Namespace) error {
	return m.updateErr
}

func (m *mockCatalog) Remove(_ context.Context, _ domain.Namespace) error {
	return m.removeErr
}

func (m *mockCatalog) List(_ context.Context) ([]domain.Arrow, error) {
	return m.listArrows, m.listErr
}

func (m *mockCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return m.getArrow, m.getErr
}

func (m *mockCatalog) IsInstalled(_ context.Context, _ domain.Namespace) bool {
	return m.isInstalled
}

func (m *mockCatalog) AddWithManifest(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.ArrowManifest,
) error {
	return m.addErr
}

func (m *mockCatalog) UpdateWithManifest(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.ArrowManifest,
) error {
	return m.updateErr
}

// --- mock deps ---

type mockDeps struct {
	resolveErr       error
	resolvePlan      appDeps.Plan
	executeErr       error
	hasDependents    bool
	hasDependentsErr error
	orphans          []domain.Namespace
	orphansErr       error
	diffResult       appDeps.DepDiff
}

func (m *mockDeps) Resolve(
	_ context.Context,
	_ domain.Namespace,
) (appDeps.Plan, error) {
	return m.resolvePlan, m.resolveErr
}

func (m *mockDeps) Execute(
	_ context.Context,
	_ appDeps.Plan,
) error {
	return m.executeErr
}

func (m *mockDeps) Unplan(
	_ context.Context,
	_ domain.Namespace,
) (appDeps.Plan, error) {
	return nil, nil
}

func (m *mockDeps) HasDependents(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) (bool, error) {
	return m.hasDependents, m.hasDependentsErr
}

func (m *mockDeps) Orphans(
	_ context.Context,
	_ domain.Namespace,
) ([]domain.Namespace, error) {
	return m.orphans, m.orphansErr
}

func (m *mockDeps) DiffDeps(
	_ *domain.ArrowManifest,
	_ *domain.ArrowManifest,
) appDeps.DepDiff {
	return m.diffResult
}

// --- mock execution ---

type mockExecution struct {
	beginExecutionErr error
	stopErr           error
	installErr        error
	uninstallErr      error
}

func (m *mockExecution) BeginExecution(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	_ map[string]string,
) error {
	return m.beginExecutionErr
}

func (m *mockExecution) Stop(_ context.Context, _ domain.Namespace) error {
	return m.stopErr
}

func (m *mockExecution) Install(
	_ context.Context,
	_ domain.Namespace,
	_ map[string]string,
) error {
	return m.installErr
}

func (m *mockExecution) Uninstall(
	_ context.Context,
	_ domain.Namespace,
	_ map[string]string,
) error {
	return m.uninstallErr
}

// --- helpers ---

func makeTestManifest(name string) *domain.ArrowManifest {
	return &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{
			Name:        name,
			Version:     "1.0.0",
			Description: "A test arrow",
			Tags:        []string{"test"},
		},
	}
}

func newTestService(
	t *testing.T,
	cat catalog.Catalog,
	exc execution.Execution,
	v vault.Vault,
) *arrowService {
	t.Helper()
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	return &arrowService{
		catalog:         cat,
		deps:            &mockDeps{},
		execution:       exc,
		resolveManifest: func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) { return nil, nil },
		asynxRuntime:    axRuntime,
		vault:           v,
	}
}

func newTestServiceWithDeps(
	t *testing.T,
	cat catalog.Catalog,
	exc execution.Execution,
	d appDeps.Deps,
	resolve manifest.ResolveFunc,
) *arrowService {
	t.Helper()
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	if resolve == nil {
		resolve = func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) { return nil, nil }
	}
	return &arrowService{
		catalog:         cat,
		deps:            d,
		execution:       exc,
		resolveManifest: resolve,
		asynxRuntime:    axRuntime,
	}
}

// newTestServiceWithRuntime constructs a service with a custom asynxRuntime.
func newTestServiceWithRuntime(
	t *testing.T,
	cat catalog.Catalog,
	exc execution.Execution,
	rt asynx.Asynx[domainRuntime.ArrowRuntime],
) *arrowService {
	t.Helper()
	return &arrowService{
		catalog:         cat,
		deps:            &mockDeps{},
		execution:       exc,
		resolveManifest: func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) { return nil, nil },
		asynxRuntime:    rt,
	}
}

// manifoldIface matches manifold.Manifold for test wiring.
type manifoldIface interface {
	ResolveArrow(context.Context, domain.Namespace) (*domain.ArrowManifest, error)
	ResolveQuiver(context.Context, domain.Namespace) (*domain.QuiverManifest, error)
	ParseArrow([]byte) (*domain.ArrowManifest, error)
}

type mockManifold struct {
	parseManifest *domain.ArrowManifest
	parseErr      error
}

func (m *mockManifold) ResolveArrow(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) {
	return nil, errors.New("not used in these tests")
}

func (m *mockManifold) ResolveQuiver(_ context.Context, _ domain.Namespace) (*domain.QuiverManifest, error) {
	return nil, errors.New("not used in these tests")
}

func (m *mockManifold) ParseArrow(_ []byte) (*domain.ArrowManifest, error) {
	return m.parseManifest, m.parseErr
}

func newTestServiceWithManifold(t *testing.T, cat catalog.Catalog, m manifoldIface) *arrowService {
	t.Helper()
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	return &arrowService{
		catalog:         cat,
		deps:            &mockDeps{},
		manifold:        m,
		execution:       &mockExecution{},
		resolveManifest: func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) { return nil, nil },
		asynxRuntime:    axRuntime,
	}
}

// --- Add delegates to catalog ---

func TestAdd_DelegatesToCatalog_ReturnsError(t *testing.T) {
	cat := &mockCatalog{addErr: apperrors.ErrInvalidNamespace}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	err := svc.Add(context.Background(), "bad-namespace")
	require.Error(t, err)
}

func TestAdd_DelegatesToCatalog_Success(t *testing.T) {
	cat := &mockCatalog{}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
}

// --- Update delegates to catalog ---

func TestUpdate_DelegatesToCatalog_ReturnsError(t *testing.T) {
	cat := &mockCatalog{getErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

func TestUpdate_DelegatesToCatalog_Success(t *testing.T) {
	existing := &domain.Arrow{Namespace: "github.com/org/repo"}
	cat := &mockCatalog{getArrow: existing}
	man := makeTestManifest("arrow")
	resolve := func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) {
		return man, nil
	}
	svc := newTestServiceWithDeps(t, cat, &mockExecution{}, &mockDeps{}, resolve)

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
}

// --- Remove delegates to catalog ---

func TestRemove_DelegatesToCatalog_ReturnsError(t *testing.T) {
	cat := &mockCatalog{removeErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

func TestRemove_DelegatesToCatalog_Success(t *testing.T) {
	cat := &mockCatalog{}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
}

// --- List: DTO mapping and state enrichment ---

func TestList_EmptyCatalog_ReturnsEmpty(t *testing.T) {
	cat := &mockCatalog{listArrows: []domain.Arrow{}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_CatalogError_ReturnsError(t *testing.T) {
	cat := &mockCatalog{listErr: errors.New("db error")}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	_, err := svc.List(context.Background())
	require.Error(t, err)
}

func TestList_MapsArrowToDTO(t *testing.T) {
	man := makeTestManifest("Arrow")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: "github.com/org/repo", Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}},
		},
	}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result[0].Namespace)
	assert.Equal(t, "Arrow", result[0].Name)
	assert.Equal(t, "1.0.0", result[0].Version)
	assert.Equal(t, "A test arrow", result[0].Description)
	assert.Equal(t, []string{"test"}, result[0].Tags)
}

func TestList_NoRuntime_UsesAbsentState(t *testing.T) {
	man := makeTestManifest("Arrow")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: "github.com/org/repo", Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}},
		},
	}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.ArrowStateAbsent, result[0].State)
}

func TestList_WithRuntimeState_UsesRuntimeState(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}},
		},
	}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.ArrowStateReady, result[0].State)
}

// --- Get ---

func TestGet_CatalogErrNotFound_ReturnsErrNotFound(t *testing.T) {
	cat := &mockCatalog{getErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	got, err := svc.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestGet_CatalogError_PropagatesError(t *testing.T) {
	someErr := errors.New("unexpected db error")
	cat := &mockCatalog{getErr: someErr}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	got, err := svc.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, someErr)
	assert.Nil(t, got)
}

func TestGet_ArrowExists_ReturnsArrow(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	arrow := &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}
	cat := &mockCatalog{getArrow: arrow}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	got, err := svc.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, ns, got.Namespace)
}

// --- GetDetail ---

func TestGetDetail_CatalogErrNotFound_ReturnsErrNotFound(t *testing.T) {
	cat := &mockCatalog{getErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	_, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetDetail_CatalogReturnsNil_ReturnsErrNotFound(t *testing.T) {
	cat := &mockCatalog{getArrow: nil}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	_, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetDetail_NoRuntime_ReturnsAbsentState(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, dto.State)
}

func TestGetDetail_WithRuntime_ReturnsCorrectState(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, dto.State)
}

func TestGetDetail_WithRuntimeExecution_PopulatesActiveRunAndLastReturn(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	activeRun := &domainRuntime.Execution{
		Method:    "run",
		Variables: map[string]string{"key": "value"},
	}
	lastReturn := &domainRuntime.Return{
		Method:  "run",
		Outcome: domainRuntime.ExecutionOutcomeSuccess,
	}

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmdWithExecution{
		NS:         ns,
		State:      domain.ArrowStateReady,
		Execution:  activeRun,
		LastReturn: lastReturn,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, dto.State)
	assert.Equal(t, activeRun, dto.ActiveRun)
	assert.Equal(t, lastReturn, dto.LastReturn)
}

func TestGetDetail_WithVaultEntry_ReturnsManifest(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")

	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}}
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{
			Manifest: man,
		},
	}
	svc := newTestService(t, cat, &mockExecution{}, mv)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, man.ArrowMeta.Name, dto.Manifest.Name)
}

func TestGetDetail_VaultError_StillReturnsDTO(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}}
	mv := &mocks.Vault{GetArrowErr: errors.New("vault unavailable")}
	svc := newTestService(t, cat, &mockExecution{}, mv)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, ns, dto.Namespace)
}

func TestGetDetail_NilVault_StillReturnsDTO(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, dto)
}

// --- HasDependents ---

func TestHasDependents_DelegatesToDeps_ReturnsFalse(t *testing.T) {
	d := &mockDeps{hasDependents: false}
	svc := newTestServiceWithDeps(t, &mockCatalog{}, &mockExecution{}, d, nil)

	has, err := svc.HasDependents(context.Background(), "github.com/org/dep", "")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasDependents_DelegatesToDeps_ReturnsTrue(t *testing.T) {
	d := &mockDeps{hasDependents: true}
	svc := newTestServiceWithDeps(t, &mockCatalog{}, &mockExecution{}, d, nil)

	has, err := svc.HasDependents(context.Background(), "github.com/org/dep", "")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasDependents_DelegatesToDeps_ReturnsError(t *testing.T) {
	d := &mockDeps{hasDependentsErr: errors.New("db error")}
	svc := newTestServiceWithDeps(t, &mockCatalog{}, &mockExecution{}, d, nil)

	has, err := svc.HasDependents(context.Background(), "github.com/org/dep", "")
	require.Error(t, err)
	assert.False(t, has)
}

// --- Install ---

func TestInstall_NoDeps_DelegatesToExecution_Success(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	d := &mockDeps{}
	svc := newTestServiceWithDeps(t, cat, exc, d, nil)

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.NoError(t, err)
}

func TestInstall_DepsResolveError_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	d := &mockDeps{resolveErr: errors.New("resolve failed")}
	svc := newTestServiceWithDeps(t, cat, exc, d, nil)

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
}

func TestInstall_ExecutionError_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{installErr: apperrors.ErrNotFound}
	d := &mockDeps{}
	svc := newTestServiceWithDeps(t, cat, exc, d, nil)

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestInstall_MissingDepsResolveFails_ReturnsError(t *testing.T) {
	dep := domain.Namespace("github.com/org/dep")
	cat := &mockCatalog{isInstalled: false}
	exc := &mockExecution{}
	d := &mockDeps{
		resolvePlan: appDeps.Plan{{Namespace: dep, Type: domain.ToolDep}},
	}
	resolve := func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) {
		return nil, errors.New("manifest not found")
	}
	svc := newTestServiceWithDeps(t, cat, exc, d, resolve)

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
}

// --- Uninstall ---

func TestUninstall_NoDependents_DelegatesToExecution_Success(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	d := &mockDeps{hasDependents: false}
	svc := newTestServiceWithDeps(t, cat, exc, d, nil)

	err := svc.Uninstall(context.Background(), "github.com/org/repo", nil)
	require.NoError(t, err)
}

func TestUninstall_HasDependents_ReturnsDependentsExistError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	d := &mockDeps{hasDependents: true}
	svc := newTestServiceWithDeps(t, cat, exc, d, nil)

	err := svc.Uninstall(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrDependentsExist)
}

func TestUninstall_HasDependentsError_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	d := &mockDeps{hasDependentsErr: errors.New("db error")}
	svc := newTestServiceWithDeps(t, cat, exc, d, nil)

	err := svc.Uninstall(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
}

func TestUninstall_ExecutionError_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{uninstallErr: apperrors.ErrStateViolation}
	d := &mockDeps{hasDependents: false}
	svc := newTestServiceWithDeps(t, cat, exc, d, nil)

	err := svc.Uninstall(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// --- BeginExecution ---

func TestBeginExecution_DelegatesToExecution_Success(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	svc := newTestService(t, cat, exc, nil)

	err := svc.BeginExecution(context.Background(), "github.com/org/repo", "_execute", nil)
	require.NoError(t, err)
}

func TestBeginExecution_DelegatesToExecution_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{beginExecutionErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, exc, nil)

	err := svc.BeginExecution(context.Background(), "github.com/org/repo", "_execute", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// --- Stop ---

func TestStop_DelegatesToExecution_Success(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	svc := newTestService(t, cat, exc, nil)

	err := svc.Stop(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
}

func TestStop_DelegatesToExecution_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{stopErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, exc, nil)

	err := svc.Stop(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// --- Runtime error branches in List and GetDetail ---

func TestList_RuntimeGetError_ReturnsError(t *testing.T) {
	man := makeTestManifest("Arrow")
	storeErr := errors.New("store failure")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: "github.com/org/repo", Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}},
		},
	}
	rt := &errAsynxRuntime{getErr: storeErr}
	svc := newTestServiceWithRuntime(t, cat, &mockExecution{}, rt)

	_, err := svc.List(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, storeErr)
}

func TestGetDetail_CatalogNonNotFoundError_PropagatesError(t *testing.T) {
	someErr := errors.New("unexpected db error")
	cat := &mockCatalog{getErr: someErr}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	_, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, someErr)
}

func TestGetDetail_RuntimeGetError_ReturnsError(t *testing.T) {
	man := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	storeErr := errors.New("store failure")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Versions: map[string]domain.ArrowManifest{domain.VersionLatestRef: {ArrowMeta: man.ArrowMeta}}}}
	rt := &errAsynxRuntime{getErr: storeErr}
	svc := newTestServiceWithRuntime(t, cat, &mockExecution{}, rt)

	_, err := svc.GetDetail(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, storeErr)
}

// --- Seed ---

func TestSeed_InvalidNamespace_ReturnsError(t *testing.T) {
	svc := newTestServiceWithManifold(t, &mockCatalog{}, &mockManifold{})
	err := svc.Seed(context.Background(), "bad", []byte("yaml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestSeed_ManifoldParseError_ReturnsInvalidManifestError(t *testing.T) {
	mo := &mockManifold{parseErr: errors.New("parse failed")}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, mo)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("bad"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidManifest)
}

func TestSeed_CatalogError_ReturnsError(t *testing.T) {
	man := makeTestManifest("arrow")
	mo := &mockManifold{parseManifest: man}
	cat := &mockCatalog{addErr: errors.New("storage failure")}
	svc := newTestServiceWithManifold(t, cat, mo)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.Error(t, err)
}

func TestSeed_AlreadyExists_UpdatesManifest(t *testing.T) {
	man := makeTestManifest("arrow")
	mo := &mockManifold{parseManifest: man}
	cat := &mockCatalog{addErr: apperrors.ErrAlreadyExists}
	svc := newTestServiceWithManifold(t, cat, mo)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
}

func TestSeed_AlreadyExists_UpdateError_ReturnsError(t *testing.T) {
	man := makeTestManifest("arrow")
	mo := &mockManifold{parseManifest: man}
	cat := &mockCatalog{addErr: apperrors.ErrAlreadyExists, updateErr: errors.New("update failed")}
	svc := newTestServiceWithManifold(t, cat, mo)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.Error(t, err)
}

func TestSeed_Success_DelegatesToCatalogAddWithManifest(t *testing.T) {
	man := makeTestManifest("arrow")
	mo := &mockManifold{parseManifest: man}
	cat := &mockCatalog{}
	svc := newTestServiceWithManifold(t, cat, mo)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
}

// --- ValidateManifest ---

func TestValidateManifest_ManifoldSuccess_ReturnsValidTrue(t *testing.T) {
	man := makeTestManifest("arrow")
	mo := &mockManifold{parseManifest: man}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, mo)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidateManifest_RuleErrors_ReturnsValidFalseWithErrors(t *testing.T) {
	asmErrs := ruleset.RuleErrors{
		{Field: "lifecycle.install", Rule: "missing_pair", Message: "install requires uninstall"},
	}
	mo := &mockManifold{parseErr: asmErrs}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, mo)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "lifecycle.install", result.Errors[0].Field)
	assert.Equal(t, "missing_pair", result.Errors[0].Rule)
}

func TestValidateManifest_TranslatorError_ReturnsValidFalseWithParseError(t *testing.T) {
	mo := &mockManifold{parseErr: errors.New("unknown schema")}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, mo)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "parse_error", result.Errors[0].Rule)
	assert.Contains(t, result.Errors[0].Message, "unknown schema")
}

func TestValidateManifest_Success_PopulatesSupportedPlatforms(t *testing.T) {
	man := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "arrow", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64:  {},
			domain.OSDarwinARM64: {},
		},
	}
	mo := &mockManifold{parseManifest: man}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, mo)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
	require.Len(t, result.SupportedPlatforms, 2)
	assert.Contains(t, result.SupportedPlatforms, domain.OSLinuxAMD64)
	assert.Contains(t, result.SupportedPlatforms, domain.OSDarwinARM64)
}

func TestValidateManifest_Success_PopulatesUnsupportedPlatforms(t *testing.T) {
	man := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "arrow", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
	}
	mo := &mockManifold{parseManifest: man}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, mo)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
	require.Len(t, result.UnsupportedPlatforms, 5)
	assert.NotContains(t, result.UnsupportedPlatforms, domain.OSLinuxAMD64)
	assert.Contains(t, result.UnsupportedPlatforms, domain.OSLinuxARM64)
	assert.Contains(t, result.UnsupportedPlatforms, domain.OSWindowsAMD64)
	assert.Contains(t, result.UnsupportedPlatforms, domain.OSWindowsARM64)
	assert.Contains(t, result.UnsupportedPlatforms, domain.OSDarwinAMD64)
	assert.Contains(t, result.UnsupportedPlatforms, domain.OSDarwinARM64)
}

func TestValidateManifest_RuleErrors_PopulatesPlatformsAsEmpty(t *testing.T) {
	asmErrs := ruleset.RuleErrors{
		{Field: "lifecycle.install", Rule: "missing_pair", Message: "install requires uninstall"},
	}
	mo := &mockManifold{parseErr: asmErrs}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, mo)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
	assert.Empty(t, result.SupportedPlatforms)
	assert.Empty(t, result.UnsupportedPlatforms)
}
