package arrow

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
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

func (e *errAsynxRuntime) WaitPublish() {}

// --- mock catalog ---

type mockCatalog struct {
	addErr           error
	updateErr        error
	removeErr        error
	listArrows       []domain.Arrow
	listErr          error
	getArrow         *domain.Arrow
	getErr           error
	hasDependents    bool
	hasDependentsErr error
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

func (m *mockCatalog) HasDependents(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) (bool, error) {
	return m.hasDependents, m.hasDependentsErr
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
		Name:        name,
		Version:     "1.0.0",
		Description: "A test arrow",
		Tags:        []string{"test"},
	}
}

func newTestService(t *testing.T, cat catalog.Catalog, exc execution.Execution, v vault.Vault) *arrowService {
	t.Helper()
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	return &arrowService{
		catalog:      cat,
		execution:    exc,
		asynxRuntime: axRuntime,
		vault:        v,
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
		catalog:      cat,
		execution:    exc,
		asynxRuntime: rt,
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
	cat := &mockCatalog{updateErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

func TestUpdate_DelegatesToCatalog_Success(t *testing.T) {
	cat := &mockCatalog{}
	svc := newTestService(t, cat, &mockExecution{}, nil)

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
	manifest := makeTestManifest("Arrow")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: "github.com/org/repo", Manifest: *manifest},
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
	manifest := makeTestManifest("Arrow")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: "github.com/org/repo", Manifest: *manifest},
		},
	}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.ArrowStateAbsent, result[0].State)
}

func TestList_WithRuntimeState_UsesRuntimeState(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: ns, Manifest: *manifest},
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
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	arrow := &domain.Arrow{Namespace: ns, Manifest: *manifest}
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
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Manifest: *manifest}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, dto.State)
}

func TestGetDetail_WithRuntime_ReturnsCorrectState(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Manifest: *manifest}}
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
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Manifest: *manifest}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	activeRun := &domainRuntime.RunRecord{
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
		ActiveRun:  activeRun,
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

func TestGetDetail_WithIndirectDeps_IncludesDeps(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	indirectDeps := []domain.Namespace{"github.com/dep/one", "github.com/dep/two"}

	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Manifest: *manifest}}
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{
			Manifest:             manifest,
			IndirectDependencies: indirectDeps,
		},
	}
	svc := newTestService(t, cat, &mockExecution{}, mv)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, indirectDeps, dto.IndirectDependencies)
}

func TestGetDetail_VaultError_StillReturnsDTO(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Manifest: *manifest}}
	mv := &mocks.Vault{GetArrowErr: errors.New("vault unavailable")}
	svc := newTestService(t, cat, &mockExecution{}, mv)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Nil(t, dto.IndirectDependencies)
	assert.Equal(t, ns, dto.Namespace)
}

func TestGetDetail_NilVault_StillReturnsDTO(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Manifest: *manifest}}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Nil(t, dto.IndirectDependencies)
}

// --- HasDependents ---

func TestHasDependents_DelegatesToCatalog_ReturnsFalse(t *testing.T) {
	cat := &mockCatalog{hasDependents: false}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	has, err := svc.HasDependents(context.Background(), "github.com/org/dep", "")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasDependents_DelegatesToCatalog_ReturnsTrue(t *testing.T) {
	cat := &mockCatalog{hasDependents: true}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	has, err := svc.HasDependents(context.Background(), "github.com/org/dep", "")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasDependents_DelegatesToCatalog_ReturnsError(t *testing.T) {
	cat := &mockCatalog{hasDependentsErr: errors.New("db error")}
	svc := newTestService(t, cat, &mockExecution{}, nil)

	has, err := svc.HasDependents(context.Background(), "github.com/org/dep", "")
	require.Error(t, err)
	assert.False(t, has)
}

// --- Install ---

func TestInstall_DelegatesToExecution_Success(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	svc := newTestService(t, cat, exc, nil)

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.NoError(t, err)
}

func TestInstall_DelegatesToExecution_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{installErr: apperrors.ErrNotFound}
	svc := newTestService(t, cat, exc, nil)

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// --- Uninstall ---

func TestUninstall_DelegatesToExecution_Success(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{}
	svc := newTestService(t, cat, exc, nil)

	err := svc.Uninstall(context.Background(), "github.com/org/repo", nil)
	require.NoError(t, err)
}

func TestUninstall_DelegatesToExecution_ReturnsError(t *testing.T) {
	cat := &mockCatalog{}
	exc := &mockExecution{uninstallErr: apperrors.ErrStateViolation}
	svc := newTestService(t, cat, exc, nil)

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
	manifest := makeTestManifest("Arrow")
	storeErr := errors.New("store failure")
	cat := &mockCatalog{
		listArrows: []domain.Arrow{
			{Namespace: "github.com/org/repo", Manifest: *manifest},
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
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	storeErr := errors.New("store failure")
	cat := &mockCatalog{getArrow: &domain.Arrow{Namespace: ns, Manifest: *manifest}}
	rt := &errAsynxRuntime{getErr: storeErr}
	svc := newTestServiceWithRuntime(t, cat, &mockExecution{}, rt)

	_, err := svc.GetDetail(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, storeErr)
}
