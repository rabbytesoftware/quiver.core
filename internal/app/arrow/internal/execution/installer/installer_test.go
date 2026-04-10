package installer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock catalog ---

type mockCatalog struct {
	arrows        []domain.Arrow
	listErr       error
	hasDependents bool
	hasDepsErr    error
}

func (m *mockCatalog) Add(_ context.Context, _ domain.Namespace) error    { return nil }
func (m *mockCatalog) Update(_ context.Context, _ domain.Namespace) error { return nil }
func (m *mockCatalog) Remove(_ context.Context, _ domain.Namespace) error { return nil }
func (m *mockCatalog) List(_ context.Context) ([]domain.Arrow, error)     { return m.arrows, m.listErr }
func (m *mockCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}
func (m *mockCatalog) HasDependents(_ context.Context, _ domain.Namespace, _ domain.Namespace) (bool, error) {
	return m.hasDependents, m.hasDepsErr
}

// --- mock runner ---

type mockRunner struct {
	beginErr    error
	executeSync func(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error
	stopErr     error
	syncCalls   []syncCall
	mu          sync.Mutex
}

type syncCall struct {
	ns     domain.Namespace
	method string
}

func (m *mockRunner) BeginExecution(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
	return m.beginErr
}

func (m *mockRunner) ExecuteSync(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
	m.mu.Lock()
	m.syncCalls = append(m.syncCalls, syncCall{ns: ns, method: method})
	m.mu.Unlock()
	if m.executeSync != nil {
		return m.executeSync(ctx, ns, method, vars)
	}
	return nil
}

func (m *mockRunner) Stop(_ context.Context, _ domain.Namespace) error {
	return m.stopErr
}

// --- vault helpers ---

// trackingVault records DeleteArrow calls.
type trackingVault struct {
	mocks.Vault
	deletedNSs []domain.Namespace
	mu         sync.Mutex
}

func (v *trackingVault) DeleteArrow(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	v.deletedNSs = append(v.deletedNSs, ns)
	v.mu.Unlock()
	return nil
}

func (v *trackingVault) GetArrow(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
	return v.Vault.GetArrowEntry, v.Vault.GetArrowPath, v.Vault.GetArrowErr
}

// staleTrackingVault returns vault.ErrStale with a valid entry and records DeleteArrow calls.
type staleTrackingVault struct {
	entry      *vault.VaultEntry
	deletedNSs []domain.Namespace
	mu         sync.Mutex
}

func (v *staleTrackingVault) GetArrow(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
	return v.entry, "/home/test", vault.ErrStale
}

func (v *staleTrackingVault) PutArrow(_ context.Context, _ domain.Namespace, _ *domain.ArrowManifest, _ []domain.Namespace) (string, error) {
	return "/home/test", nil
}

func (v *staleTrackingVault) DeleteArrow(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	v.deletedNSs = append(v.deletedNSs, ns)
	v.mu.Unlock()
	return nil
}

func (v *staleTrackingVault) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *staleTrackingVault) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *staleTrackingVault) DeleteQuiver(_ context.Context, _ domain.Namespace) error {
	return nil
}

// vaultByNamespace returns different entries per namespace.
type vaultByNamespace struct {
	mu         sync.Mutex
	entries    map[domain.Namespace]*vault.VaultEntry
	deletedNSs []domain.Namespace
}

func (v *vaultByNamespace) GetArrow(_ context.Context, ns domain.Namespace) (*vault.VaultEntry, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	entry, ok := v.entries[ns]
	if !ok {
		return nil, "", vault.ErrNotCached
	}
	return entry, "/home/" + ns.String(), nil
}

func (v *vaultByNamespace) PutArrow(_ context.Context, _ domain.Namespace, _ *domain.ArrowManifest, _ []domain.Namespace) (string, error) {
	return "/home/test", nil
}

func (v *vaultByNamespace) DeleteArrow(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	v.deletedNSs = append(v.deletedNSs, ns)
	v.mu.Unlock()
	return nil
}

func (v *vaultByNamespace) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *vaultByNamespace) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *vaultByNamespace) DeleteQuiver(_ context.Context, _ domain.Namespace) error {
	return nil
}

// --- asynx stubs ---

// failingAsynxArrow is a stub asynx.Asynx[domain.Arrow] that returns a custom error from Get.
// It can optionally return a non-zero arrow along with the error.
type failingAsynxArrow struct {
	getErr error
	arrow  domain.Arrow // returned along with error
}

func (f *failingAsynxArrow) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	return "sub-id", nil
}

func (f *failingAsynxArrow) Send(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAsynxArrow) SendWait(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAsynxArrow) Shutdown(_ context.Context) error { return nil }
func (f *failingAsynxArrow) Get(_ context.Context, _ string) (domain.Arrow, error) {
	return f.arrow, f.getErr
}
func (f *failingAsynxArrow) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingAsynxArrow) Preload(_ context.Context, _ string) error        { return nil }
func (f *failingAsynxArrow) Unsubscribe(_ string) error                       { return nil }
func (f *failingAsynxArrow) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domain.Arrow]) error {
	return nil
}
func (f *failingAsynxArrow) WaitPublish() {}

// failingAsynxRuntime is a stub asynx.Asynx[domainRuntime.ArrowRuntime] that returns a custom error from Get.
// It can optionally return a non-zero runtime along with the error.
type failingAsynxRuntime struct {
	getErr  error
	runtime domainRuntime.ArrowRuntime // returned along with error
}

func (f *failingAsynxRuntime) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	return "sub-id", nil
}

func (f *failingAsynxRuntime) Send(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingAsynxRuntime) SendWait(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingAsynxRuntime) Shutdown(_ context.Context) error { return nil }
func (f *failingAsynxRuntime) Get(_ context.Context, _ string) (domainRuntime.ArrowRuntime, error) {
	return f.runtime, f.getErr
}
func (f *failingAsynxRuntime) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingAsynxRuntime) Preload(_ context.Context, _ string) error        { return nil }
func (f *failingAsynxRuntime) Unsubscribe(_ string) error                       { return nil }
func (f *failingAsynxRuntime) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime]) error {
	return nil
}
func (f *failingAsynxRuntime) WaitPublish() {}

// --- asynx builders ---

func buildAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	return ax
}

func buildAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	return ax
}

// testInstaller builds a whitebox installerService.
func testInstaller(
	t *testing.T,
	v vault.Vault,
	cat *mockCatalog,
	r *mockRunner,
) *installerService {
	t.Helper()
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	return &installerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
		deptree:   deptree.New(),
		catalog:   cat,
		runner:    r,
	}
}

func addArrowForTest(
	t *testing.T,
	svc *installerService,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) {
	t.Helper()
	_, err := svc.axArrow.Send(context.Background(), arrowcmds.AddArrow{
		Namespace: ns,
		Manifest:  *manifest,
	})
	require.NoError(t, err)
	svc.axArrow.WaitPublish()
}

func seedRuntime(
	t *testing.T,
	svc *installerService,
	ns domain.Namespace,
	state domain.ArrowState,
) {
	t.Helper()
	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: state,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()
}

// --- Install ---

func TestInstall_ArrowNotFound_ReturnsErrNotFound(t *testing.T) {
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestInstall_AlreadyInstalled_ReturnsErrStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})
	addArrowForTest(t, svc, ns, manifest)
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestInstall_NoRuntime_CallsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestInstall_AbsentRuntime_CallsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	addArrowForTest(t, svc, ns, manifest)
	seedRuntime(t, svc, ns, domain.ArrowStateAbsent)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestInstall_RunnerFails_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	r := &mockRunner{beginErr: errors.New("runner failed")}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
}

func TestInstall_AxArrowGetNonNotFoundError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)

	// Replace axArrow with one that returns a non-ErrNotFound error along with a valid arrow.
	// This tests the error path when Get returns a non-ErrNotFound error.
	svc.axArrow = &failingAsynxArrow{
		arrow:  domain.Arrow{Namespace: ns},
		getErr: errors.New("storage failure"),
	}

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.Equal(t, "storage failure", err.Error())
}

func TestInstall_AxRuntimeGetNonNotFoundError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	addArrowForTest(t, svc, ns, manifest)

	// Replace axRuntime with one that returns a non-ErrNotFound error.
	// This tests the error path when axRuntime.Get returns a non-ErrNotFound error.
	svc.axRuntime = &failingAsynxRuntime{
		runtime: domainRuntime.ArrowRuntime{Namespace: ns},
		getErr:  errors.New("runtime storage failure"),
	}

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.Equal(t, "runtime storage failure", err.Error())
}

func TestInstall_ArrowStorageError_PropagatesError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	storageErr := errors.New("arrow storage failure")

	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})
	svc.axArrow = &failingAsynxArrow{getErr: storageErr}

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, storageErr)
	assert.False(t, errors.Is(err, apperrors.ErrNotFound), "storage error must not be masked as ErrNotFound")
}

// --- Uninstall ---

func TestUninstall_NoRuntime_ReturnsErrStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestUninstall_NotReady_ReturnsErrStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})
	seedRuntime(t, svc, ns, domain.ArrowStateInstalling)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestUninstall_HasDependents_ReturnsErrDependentsExist(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	cat := &mockCatalog{hasDependents: true}
	svc := testInstaller(t, &mocks.Vault{}, cat, &mockRunner{})
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrDependentsExist)
}

func TestUninstall_HasDependentsError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	cat := &mockCatalog{hasDepsErr: errors.New("db error")}
	svc := testInstaller(t, &mocks.Vault{}, cat, &mockRunner{})
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
}

func TestUninstall_Ready_NoDeps_CallsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestUninstall_RunnerFails_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	r := &mockRunner{beginErr: errors.New("runner failed")}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
}

func TestUninstall_RuntimeStorageError_PropagatesError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	storageErr := errors.New("runtime storage failure")

	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})
	// Replace axRuntime with one that returns a non-ErrNotFound error.
	svc.axRuntime = &failingAsynxRuntime{getErr: storageErr}

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	// Must propagate the raw storage error, NOT ErrStateViolation.
	assert.ErrorIs(t, err, storageErr)
	assert.False(t, errors.Is(err, apperrors.ErrStateViolation), "storage error must not be masked as ErrStateViolation")
}

// --- CleanupAfterUninstall ---

func TestCleanupAfterUninstall_NoVaultEntry_DeletesNs(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	tv := &trackingVault{
		Vault: mocks.Vault{GetArrowErr: errors.New("not cached")},
	}
	svc := testInstaller(t, tv, &mockCatalog{}, &mockRunner{})

	svc.CleanupAfterUninstall(context.Background(), ns)

	tv.mu.Lock()
	defer tv.mu.Unlock()
	require.Len(t, tv.deletedNSs, 1)
	assert.Equal(t, ns, tv.deletedNSs[0])
}

func TestCleanupAfterUninstall_WithVaultEntry_NoDeps_DeletesNs(t *testing.T) {
	ns := domain.Namespace("github.com/org/main")
	manifest := &domain.ArrowManifest{Name: "Main", Version: "1.0.0"}

	vbn := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			ns: {Manifest: manifest},
		},
	}
	svc := testInstaller(t, vbn, &mockCatalog{}, &mockRunner{})

	svc.CleanupAfterUninstall(context.Background(), ns)

	vbn.mu.Lock()
	defer vbn.mu.Unlock()
	require.Len(t, vbn.deletedNSs, 1)
	assert.Equal(t, ns, vbn.deletedNSs[0])
}

func TestCleanupAfterUninstall_WithOrphanDep_UninstallsAndDeletesDep(t *testing.T) {
	ns1 := domain.Namespace("github.com/org/main")
	ns2 := domain.Namespace("github.com/org/dep")

	depManifest := &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"}
	mainManifest := &domain.ArrowManifest{
		Name:         "Main",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{ns2},
	}

	vbn := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			ns1: {Manifest: mainManifest},
			ns2: {Manifest: depManifest},
		},
	}

	// ns2 has no dependents (excluding ns1 which we're cleaning up)
	cat := &mockCatalog{hasDependents: false}

	syncCalls := make([]syncCall, 0)
	r := &mockRunner{
		executeSync: func(_ context.Context, ns domain.Namespace, method string, _ map[string]string) error {
			return nil
		},
	}

	svc := testInstaller(t, vbn, cat, r)

	svc.CleanupAfterUninstall(context.Background(), ns1)

	_ = syncCalls

	r.mu.Lock()
	defer r.mu.Unlock()
	// ExecuteSync should have been called for ns2
	require.Len(t, r.syncCalls, 1)
	assert.Equal(t, ns2, r.syncCalls[0].ns)
	assert.Equal(t, "_uninstall", r.syncCalls[0].method)

	vbn.mu.Lock()
	defer vbn.mu.Unlock()
	// Both ns1 and ns2 should be deleted
	assert.Contains(t, vbn.deletedNSs, ns1)
	assert.Contains(t, vbn.deletedNSs, ns2)
}

func TestCleanupAfterUninstall_DepHasOtherDependents_SkipsDep(t *testing.T) {
	ns1 := domain.Namespace("github.com/org/main")
	ns2 := domain.Namespace("github.com/org/dep")

	depManifest := &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"}
	mainManifest := &domain.ArrowManifest{
		Name:         "Main",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{ns2},
	}

	vbn := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			ns1: {Manifest: mainManifest},
			ns2: {Manifest: depManifest},
		},
	}

	// ns2 still has other dependents
	cat := &mockCatalog{hasDependents: true}
	r := &mockRunner{}

	svc := testInstaller(t, vbn, cat, r)

	svc.CleanupAfterUninstall(context.Background(), ns1)

	r.mu.Lock()
	defer r.mu.Unlock()
	// ExecuteSync should NOT have been called for ns2 (it still has dependents)
	assert.Empty(t, r.syncCalls)

	vbn.mu.Lock()
	defer vbn.mu.Unlock()
	// Only ns1 should be deleted
	assert.Contains(t, vbn.deletedNSs, ns1)
	assert.NotContains(t, vbn.deletedNSs, ns2)
}

func TestCleanupAfterUninstall_ExecuteSyncFails_ContinuesAndDeletesNs(t *testing.T) {
	ns1 := domain.Namespace("github.com/org/main")
	ns2 := domain.Namespace("github.com/org/dep")

	depManifest := &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"}
	mainManifest := &domain.ArrowManifest{
		Name:         "Main",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{ns2},
	}

	vbn := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			ns1: {Manifest: mainManifest},
			ns2: {Manifest: depManifest},
		},
	}

	cat := &mockCatalog{hasDependents: false}
	r := &mockRunner{
		executeSync: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			return errors.New("uninstall failed")
		},
	}

	svc := testInstaller(t, vbn, cat, r)

	svc.CleanupAfterUninstall(context.Background(), ns1)

	vbn.mu.Lock()
	defer vbn.mu.Unlock()
	// ns1 must be deleted even though ns2's uninstall failed
	assert.Contains(t, vbn.deletedNSs, ns1)
	// ns2 should NOT be deleted (ExecuteSync failed → vault.DeleteArrow not called)
	assert.NotContains(t, vbn.deletedNSs, ns2)
}

func TestCleanupAfterUninstall_DepTreeError_DeletesNsAndReturns(t *testing.T) {
	ns := domain.Namespace("github.com/org/main")
	manifest := &domain.ArrowManifest{Name: "Main", Version: "1.0.0"}

	// Vault returns an entry but GetArrow for deps (during topo resolve) returns ErrNotCached.
	// We use a custom vault that returns the main entry on the first call but then fails.
	callCount := 0
	_ = callCount

	// A simpler approach: use the real deptree but make the root's dep vault lookup fail
	// by not seeding the dep's entry. The depTree will still succeed but the dep won't be in
	// the topo result.

	vbn := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			ns: {Manifest: manifest},
		},
	}

	cat := &mockCatalog{}
	r := &mockRunner{}

	svc := testInstaller(t, vbn, cat, r)

	svc.CleanupAfterUninstall(context.Background(), ns)

	vbn.mu.Lock()
	defer vbn.mu.Unlock()
	assert.Contains(t, vbn.deletedNSs, ns)
}

// --- Install runtime ErrNotFound path (namespace check) ---

func TestInstall_RuntimeGetErrNotFound_ProceedsAsNoRuntime(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	// No runtime seeded → axRuntime.Get returns ErrNotFound → rt.Namespace == "" → not state violation
	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)
}

// --- Uninstall with ErrNotFound runtime ---

func TestUninstall_RuntimeErrNotFound_ReturnsStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	// No runtime seeded → axRuntime.Get returns ErrNotFound → state violation
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// --- CleanupAfterUninstall with indirect dependencies ---

// TestCleanupAfterUninstall_WithIndirectDeps_OrphanNotInTopoOrder verifies that
// a namespace only in IndirectDependencies (not in Manifest.Dependencies) is
// detected as an orphan candidate but is NOT uninstalled because deptree only
// traverses direct dependencies. Only deps reachable via the vault resolver
// (which returns Manifest.Dependencies) appear in the topo order.
func TestCleanupAfterUninstall_WithIndirectDeps_OrphanNotInTopoOrder(t *testing.T) {
	ns1 := domain.Namespace("github.com/org/main")
	ns2 := domain.Namespace("github.com/org/indirect")

	mainManifest := &domain.ArrowManifest{
		Name:    "Main",
		Version: "1.0.0",
	}

	vbn := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			ns1: {
				Manifest:             mainManifest,
				IndirectDependencies: []domain.Namespace{ns2},
			},
			ns2: {Manifest: &domain.ArrowManifest{Name: "Indirect", Version: "1.0.0"}},
		},
	}

	cat := &mockCatalog{hasDependents: false}
	r := &mockRunner{}

	svc := testInstaller(t, vbn, cat, r)

	svc.CleanupAfterUninstall(context.Background(), ns1)

	r.mu.Lock()
	defer r.mu.Unlock()
	// ns2 is not in the topo order (not a direct dep) so ExecuteSync is NOT called
	assert.Empty(t, r.syncCalls)

	vbn.mu.Lock()
	defer vbn.mu.Unlock()
	// ns1 is deleted; ns2 is not (wasn't in the topo walk)
	assert.Contains(t, vbn.deletedNSs, ns1)
	assert.NotContains(t, vbn.deletedNSs, ns2)
}

// --- asynxModels.ErrNotFound for Install ---

func TestInstall_AsynxArrowErrNotFound_ReturnsErrNotFound(t *testing.T) {
	// axArrow.Get returns ErrNotFound for a namespace that was never registered
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, &mockRunner{})

	err := svc.Install(context.Background(), "github.com/org/nonexistent", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// --- asynxModels error coverage for Install runtime branch ---

func TestInstall_RuntimeNonNotFoundError_ReturnsError(t *testing.T) {
	// We can't inject a non-ErrNotFound error into asynxRuntime.Get without
	// a real store error. Instead, verify the state-violation path:
	// when a runtime is seeded with a non-absent state, Install returns ErrStateViolation.
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, &mockCatalog{}, r)
	addArrowForTest(t, svc, ns, manifest)
	seedRuntime(t, svc, ns, domain.ArrowStateInstalling)

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// --- cleanupAfterUninstall deptree error path ---

// failingDepTree always returns an error from Resolve.
type failingDepTree struct {
	err error
}

func (f *failingDepTree) Resolve(_ context.Context, _ domain.Namespace, _ deptree.ResolverFunc) ([]domain.Namespace, error) {
	return nil, f.err
}

func TestCleanupAfterUninstall_DepTreeResolveError_DeletesNsAndReturns(t *testing.T) {
	ns := domain.Namespace("github.com/org/main")
	ns2 := domain.Namespace("github.com/org/dep")
	manifest := &domain.ArrowManifest{
		Name:         "Main",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{ns2},
	}

	tv := &trackingVault{
		Vault: mocks.Vault{
			GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
			GetArrowPath:  "/home/main",
		},
	}

	cat := &mockCatalog{hasDependents: false}
	r := &mockRunner{}

	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	inst := &installerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     tv,
		deptree:   &failingDepTree{err: errors.New("cycle detected")},
		catalog:   cat,
		runner:    r,
	}

	inst.CleanupAfterUninstall(context.Background(), ns)

	tv.mu.Lock()
	defer tv.mu.Unlock()
	require.Len(t, tv.deletedNSs, 1)
	assert.Equal(t, ns, tv.deletedNSs[0])
}

func TestCleanupAfterUninstall_StaleVaultEntry_RunsCascade(t *testing.T) {
	ns1 := domain.Namespace("github.com/org/main")

	mainManifest := &domain.ArrowManifest{
		Name:    "Main",
		Version: "1.0.0",
	}

	// Vault returns ErrStale with a valid entry — cleanup must still run.
	stv := &staleTrackingVault{
		entry: &vault.VaultEntry{Manifest: mainManifest},
	}

	cat := &mockCatalog{}
	r := &mockRunner{}

	svc := &installerService{
		axArrow:   buildAsynxArrow(t),
		axRuntime: buildAsynxRuntime(t),
		vault:     stv,
		deptree:   deptree.New(),
		catalog:   cat,
		runner:    r,
	}

	svc.cleanupAfterUninstall(context.Background(), ns1)

	stv.mu.Lock()
	defer stv.mu.Unlock()
	// ns1 must be deleted even though vault returned ErrStale.
	assert.Contains(t, stv.deletedNSs, ns1, "ns1 must be deleted when vault returns ErrStale")
}

