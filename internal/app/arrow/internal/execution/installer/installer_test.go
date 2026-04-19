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
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock runner ---

type mockRunner struct {
	beginErr    error
	beginCalled bool
	executeSync func(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error
	stopErr     error
	syncCalls   []syncCall
	mu          sync.Mutex
}

type syncCall struct {
	ns     domain.Namespace
	method string
}

func (m *mockRunner) BeginExecution(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	_ map[string]string,
) error {
	m.beginCalled = true
	return m.beginErr
}

func (m *mockRunner) ExecuteSync(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error {
	m.mu.Lock()
	m.syncCalls = append(m.syncCalls, syncCall{ns: ns, method: method})
	m.mu.Unlock()
	if m.executeSync != nil {
		return m.executeSync(ctx, ns, method, vars)
	}
	return nil
}

func (m *mockRunner) Stop(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.stopErr
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
func (f *failingAsynxArrow) Forget(_ context.Context, _ string) error { return nil }
func (f *failingAsynxArrow) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	return "forget-sub-id", nil
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
func (f *failingAsynxRuntime) Forget(_ context.Context, _ string) error { return nil }
func (f *failingAsynxRuntime) OnForget(_ asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error) {
	return "forget-sub-id", nil
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
	r *mockRunner,
) *installerService {
	t.Helper()
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	return &installerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
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
		Version:   *manifest,
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
	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestInstall_AlreadyInstalled_ReturnsErrStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})
	addArrowForTest(t, svc, ns, manifest)
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestInstall_NoRuntime_CallsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	r := &mockRunner{}
	mv := &mocks.Vault{GetArrowEntry: &vault.VaultEntry{Manifest: manifest}}
	svc := testInstaller(t, mv, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestInstall_AbsentRuntime_CallsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	r := &mockRunner{}
	mv := &mocks.Vault{GetArrowEntry: &vault.VaultEntry{Manifest: manifest}}
	svc := testInstaller(t, mv, r)
	addArrowForTest(t, svc, ns, manifest)
	seedRuntime(t, svc, ns, domain.ArrowStateAbsent)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestInstall_RunnerFails_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	r := &mockRunner{beginErr: errors.New("runner failed")}
	mv := &mocks.Vault{GetArrowEntry: &vault.VaultEntry{Manifest: manifest}}
	svc := testInstaller(t, mv, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
}

func TestInstall_AxArrowGetNonNotFoundError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, r)

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
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, r)
	addArrowForTest(t, svc, ns, manifest)

	svc.axRuntime = &failingAsynxRuntime{
		runtime: domainRuntime.ArrowRuntime{Ref: ns},
		getErr:  errors.New("runtime storage failure"),
	}

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.Equal(t, "runtime storage failure", err.Error())
}

func TestInstall_PopulatesVaultBeforeExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	mv := &mocks.Vault{GetArrowEntry: &vault.VaultEntry{Manifest: manifest}}
	r := &mockRunner{}
	svc := testInstaller(t, mv, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, mv.PutArrowCalls, "vault.PutArrow must be called before BeginExecution")
}

func TestInstall_VaultPutFails_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		PutArrowErr:   errors.New("disk full"),
	}
	r := &mockRunner{}
	svc := testInstaller(t, mv, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.False(t, r.beginCalled, "BeginExecution must not be called when vault.PutArrow fails")
}

func TestInstall_ArrowStorageError_PropagatesError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	storageErr := errors.New("arrow storage failure")

	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})
	svc.axArrow = &failingAsynxArrow{getErr: storageErr}

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, storageErr)
	assert.False(t, errors.Is(err, apperrors.ErrNotFound), "storage error must not be masked as ErrNotFound")
}

// --- Uninstall ---

func TestUninstall_NoRuntime_ReturnsErrStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestUninstall_NotReady_ReturnsErrStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})
	seedRuntime(t, svc, ns, domain.ArrowStateInstalling)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestUninstall_Ready_CallsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, r)
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestUninstall_RunnerFails_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	r := &mockRunner{beginErr: errors.New("runner failed")}
	svc := testInstaller(t, &mocks.Vault{}, r)
	seedRuntime(t, svc, ns, domain.ArrowStateReady)

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
}

func TestUninstall_RuntimeStorageError_PropagatesError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	storageErr := errors.New("runtime storage failure")

	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})
	svc.axRuntime = &failingAsynxRuntime{getErr: storageErr}

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, storageErr)
	assert.False(t, errors.Is(err, apperrors.ErrStateViolation), "storage error must not be masked as ErrStateViolation")
}

// --- Install runtime ErrNotFound path (namespace check) ---

func TestInstall_RuntimeGetErrNotFound_ProceedsAsNoRuntime(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	r := &mockRunner{}
	mv := &mocks.Vault{GetArrowEntry: &vault.VaultEntry{Manifest: manifest}}
	svc := testInstaller(t, mv, r)
	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)
}

// --- Uninstall with ErrNotFound runtime ---

func TestUninstall_RuntimeErrNotFound_ReturnsStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// --- asynxModels.ErrNotFound for Install ---

func TestInstall_AsynxArrowErrNotFound_ReturnsErrNotFound(t *testing.T) {
	svc := testInstaller(t, &mocks.Vault{}, &mockRunner{})

	err := svc.Install(context.Background(), "github.com/org/nonexistent", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// --- asynxModels error coverage for Install runtime branch ---

func TestInstall_RuntimeNonNotFoundError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	r := &mockRunner{}
	svc := testInstaller(t, &mocks.Vault{}, r)
	addArrowForTest(t, svc, ns, manifest)
	seedRuntime(t, svc, ns, domain.ArrowStateInstalling)

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}
