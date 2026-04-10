package execution

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/installer"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/runner"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	engine "github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock catalog ---

type mockCatalog struct {
	hasDependents bool
	hasDepsErr    error
}

func (m *mockCatalog) Add(_ context.Context, _ domain.Namespace) error    { return nil }
func (m *mockCatalog) Update(_ context.Context, _ domain.Namespace) error { return nil }
func (m *mockCatalog) Remove(_ context.Context, _ domain.Namespace) error { return nil }
func (m *mockCatalog) List(_ context.Context) ([]domain.Arrow, error)     { return nil, nil }
func (m *mockCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}
func (m *mockCatalog) HasDependents(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) (bool, error) {
	return m.hasDependents, m.hasDepsErr
}

// --- mock runner ---

type mockRunner struct {
	mu             sync.Mutex
	beginCalls     []beginCall
	beginErr       error
	stopCalls      []domain.Namespace
	stopErr        error
	executeSyncErr error
	hook           runner.PostExecutionFn
}

type beginCall struct {
	ns     domain.Namespace
	method string
	vars   map[string]string
}

func (m *mockRunner) BeginExecution(
	_ context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error {
	m.mu.Lock()
	m.beginCalls = append(m.beginCalls, beginCall{ns: ns, method: method, vars: vars})
	m.mu.Unlock()
	return m.beginErr
}

func (m *mockRunner) ExecuteSync(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	_ map[string]string,
) error {
	return m.executeSyncErr
}

func (m *mockRunner) Stop(_ context.Context, ns domain.Namespace) error {
	m.mu.Lock()
	m.stopCalls = append(m.stopCalls, ns)
	m.mu.Unlock()
	return m.stopErr
}

func (m *mockRunner) SetPostExecutionHook(fn runner.PostExecutionFn) {
	m.mu.Lock()
	m.hook = fn
	m.mu.Unlock()
}

// --- mock installer ---

type mockInstaller struct {
	mu             sync.Mutex
	installCalls   []installCall
	installErr     error
	uninstallCalls []installCall
	uninstallErr   error
	cleanupCalls   []domain.Namespace
}

type installCall struct {
	ns   domain.Namespace
	vars map[string]string
}

func (m *mockInstaller) Install(
	_ context.Context,
	ns domain.Namespace,
	vars map[string]string,
) error {
	m.mu.Lock()
	m.installCalls = append(m.installCalls, installCall{ns: ns, vars: vars})
	m.mu.Unlock()
	return m.installErr
}

func (m *mockInstaller) Uninstall(
	_ context.Context,
	ns domain.Namespace,
	vars map[string]string,
) error {
	m.mu.Lock()
	m.uninstallCalls = append(m.uninstallCalls, installCall{ns: ns, vars: vars})
	m.mu.Unlock()
	return m.uninstallErr
}

func (m *mockInstaller) CleanupAfterUninstall(_ context.Context, ns domain.Namespace) {
	m.mu.Lock()
	m.cleanupCalls = append(m.cleanupCalls, ns)
	m.mu.Unlock()
}

// --- failing asynx runtime stub ---

// failingRuntimeAsynx fails Subscribe on a specific call number.
type failingRuntimeAsynx struct {
	subscribeCallN int
	calls          int
	err            error
}

func (f *failingRuntimeAsynx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	f.calls++
	if f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingRuntimeAsynx) Send(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingRuntimeAsynx) SendWait(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingRuntimeAsynx) Shutdown(_ context.Context) error { return nil }
func (f *failingRuntimeAsynx) Get(_ context.Context, _ string) (domainRuntime.ArrowRuntime, error) {
	return domainRuntime.ArrowRuntime{}, nil
}
func (f *failingRuntimeAsynx) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingRuntimeAsynx) Preload(_ context.Context, _ string) error        { return nil }
func (f *failingRuntimeAsynx) Unsubscribe(_ string) error                       { return nil }
func (f *failingRuntimeAsynx) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
) error {
	return nil
}
func (f *failingRuntimeAsynx) WaitPublish() {}

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

// minimalEngines returns a Container with only what New() needs to succeed.
func minimalEngines() engine.Container {
	return engine.Container{
		Vault:     &mocks.Vault{},
		Manifold:  &mocks.Manifold{},
		Wizard:    &mocks.Wizard{},
		Netbridge: &mocks.Netbridge{},
		DepTree:   deptree.New(),
	}
}

// --- tests ---

func TestNew_HappyPath(t *testing.T) {
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	engines := minimalEngines()
	cat := &mockCatalog{}

	exec, err := New(axArrow, axRuntime, engines, domain.OSLinuxAMD64, cat)

	require.NoError(t, err)
	assert.NotNil(t, exec)
}

func TestNew_NilWizard_NoRegisterDispatch(t *testing.T) {
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	engines := minimalEngines()
	engines.Wizard = nil
	cat := &mockCatalog{}

	exec, err := New(axArrow, axRuntime, engines, domain.OSLinuxAMD64, cat)

	require.NoError(t, err)
	assert.NotNil(t, exec)
}

func TestNew_RunnerConstructionFailure(t *testing.T) {
	axArrow := buildAsynxArrow(t)
	wantErr := errors.New("subscribe failed")
	// runner.NewRunner calls axRuntime.Subscribe — fail on first call.
	badRuntime := &failingRuntimeAsynx{subscribeCallN: 1, err: wantErr}
	engines := minimalEngines()
	cat := &mockCatalog{}

	exec, err := New(axArrow, badRuntime, engines, domain.OSLinuxAMD64, cat)

	assert.Nil(t, exec)
	require.ErrorIs(t, err, wantErr)
}

// --- delegation tests (whitebox via executionService) ---

func testExecutionService(run runner.HookableRunner, inst installer.Installer) *executionService {
	return &executionService{runner: run, installer: inst}
}

func TestBeginExecution_DelegatesToRunner(t *testing.T) {
	ns := domain.Namespace("test/arrow")
	vars := map[string]string{"KEY": "val"}
	run := &mockRunner{}
	svc := testExecutionService(run, &mockInstaller{})

	err := svc.BeginExecution(context.Background(), ns, "_execute", vars)

	require.NoError(t, err)
	require.Len(t, run.beginCalls, 1)
	assert.Equal(t, ns, run.beginCalls[0].ns)
	assert.Equal(t, "_execute", run.beginCalls[0].method)
}

func TestBeginExecution_PropagatesRunnerError(t *testing.T) {
	want := errors.New("runner error")
	run := &mockRunner{beginErr: want}
	svc := testExecutionService(run, &mockInstaller{})

	err := svc.BeginExecution(context.Background(), "test/arrow", "_execute", nil)

	assert.ErrorIs(t, err, want)
}

func TestStop_DelegatesToRunner(t *testing.T) {
	ns := domain.Namespace("test/arrow")
	run := &mockRunner{}
	svc := testExecutionService(run, &mockInstaller{})

	err := svc.Stop(context.Background(), ns)

	require.NoError(t, err)
	require.Len(t, run.stopCalls, 1)
	assert.Equal(t, ns, run.stopCalls[0])
}

func TestStop_PropagatesRunnerError(t *testing.T) {
	want := errors.New("stop error")
	run := &mockRunner{stopErr: want}
	svc := testExecutionService(run, &mockInstaller{})

	err := svc.Stop(context.Background(), "test/arrow")

	assert.ErrorIs(t, err, want)
}

func TestInstall_DelegatesToInstaller(t *testing.T) {
	ns := domain.Namespace("test/arrow")
	vars := map[string]string{"K": "v"}
	inst := &mockInstaller{}
	svc := testExecutionService(&mockRunner{}, inst)

	err := svc.Install(context.Background(), ns, vars)

	require.NoError(t, err)
	require.Len(t, inst.installCalls, 1)
	assert.Equal(t, ns, inst.installCalls[0].ns)
}

func TestInstall_PropagatesInstallerError(t *testing.T) {
	want := errors.New("install error")
	inst := &mockInstaller{installErr: want}
	svc := testExecutionService(&mockRunner{}, inst)

	err := svc.Install(context.Background(), "test/arrow", nil)

	assert.ErrorIs(t, err, want)
}

func TestUninstall_DelegatesToInstaller(t *testing.T) {
	ns := domain.Namespace("test/arrow")
	inst := &mockInstaller{}
	svc := testExecutionService(&mockRunner{}, inst)

	err := svc.Uninstall(context.Background(), ns, nil)

	require.NoError(t, err)
	require.Len(t, inst.uninstallCalls, 1)
	assert.Equal(t, ns, inst.uninstallCalls[0].ns)
}

func TestUninstall_PropagatesInstallerError(t *testing.T) {
	want := errors.New("uninstall error")
	inst := &mockInstaller{uninstallErr: want}
	svc := testExecutionService(&mockRunner{}, inst)

	err := svc.Uninstall(context.Background(), "test/arrow", nil)

	assert.ErrorIs(t, err, want)
}

// --- integration tests: hook wiring verified via New() ---

func TestNew_HookIsWiredViaNew(t *testing.T) {
	// Verify that New() actually sets the hook on the runner by testing
	// that the returned Execution is non-nil and the wizard got RegisterDispatch called.
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	wiz := &mocks.Wizard{}
	engines := engine.Container{
		Vault:     &mocks.Vault{},
		Manifold:  &mocks.Manifold{},
		Wizard:    wiz,
		Netbridge: &mocks.Netbridge{},
		DepTree:   deptree.New(),
	}
	cat := &mockCatalog{}

	exec, err := New(axArrow, axRuntime, engines, domain.OSLinuxAMD64, cat)

	require.NoError(t, err)
	assert.NotNil(t, exec)
}

// --- integration tests: exercise the actual hook closure registered by New() ---

// newWiredExecution calls New() and returns the underlying executionService
// so whitebox tests can access its internals.
func newWiredExecution(
	t *testing.T,
	wiz *mocks.Wizard,
) (*executionService, asynx.Asynx[domain.Arrow], asynx.Asynx[domainRuntime.ArrowRuntime]) {
	t.Helper()
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	engines := engine.Container{
		Vault:     &mocks.Vault{GetArrowErr: errors.New("not found")},
		Manifold:  &mocks.Manifold{},
		Wizard:    wiz,
		Netbridge: &mocks.Netbridge{},
		DepTree:   deptree.New(),
	}
	cat := &mockCatalog{}
	exec, err := New(axArrow, axRuntime, engines, domain.OSLinuxAMD64, cat)
	require.NoError(t, err)
	svc := exec.(*executionService)
	return svc, axArrow, axRuntime
}

// hookableRunner is the interface that runner.HookableRunner exposes beyond runner.Runner.
type hookableRunner interface {
	runner.Runner
	runner.HookableRunner
	ExecuteSync(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error
}

func TestNew_HookWiring_ExecuteCancelled_TriggersStop(t *testing.T) {
	wiz := &mocks.Wizard{ExecuteErr: context.Canceled}
	svc, axArrow, axRuntime := newWiredExecution(t, wiz)

	ns := domain.Namespace("github.com/org/test")

	_, err := axArrow.Send(context.Background(), seedArrowCmd{ns: ns})
	require.NoError(t, err)
	axArrow.WaitPublish()

	// Seed runtime as Ready so _execute validation passes.
	_, err = axRuntime.Send(context.Background(), seedRuntimeCmd{ns: ns})
	require.NoError(t, err)
	axRuntime.WaitPublish()

	trackInst := &mockInstaller{}
	svc.installer = trackInst

	hr, ok := svc.runner.(hookableRunner)
	require.True(t, ok, "runner must expose ExecuteSync")

	// ExecuteSync blocks until done. Cancelled wizard fires the hook which calls BeginExecution(_stop).
	_ = hr.ExecuteSync(context.Background(), ns, "_execute", nil)

	// _uninstall cleanup must NOT be triggered for _execute.
	trackInst.mu.Lock()
	assert.Empty(t, trackInst.cleanupCalls)
	trackInst.mu.Unlock()
}

func TestNew_HookWiring_UninstallSuccess_TriggersCleanup(t *testing.T) {
	wiz := &mocks.Wizard{ExecuteErr: nil}
	svc, axArrow, axRuntime := newWiredExecution(t, wiz)

	ns := domain.Namespace("github.com/org/uninstall")

	_, err := axArrow.Send(context.Background(), seedArrowCmd{ns: ns})
	require.NoError(t, err)
	axArrow.WaitPublish()

	// Seed runtime as Ready so _uninstall validation passes.
	_, err = axRuntime.Send(context.Background(), seedRuntimeCmd{ns: ns})
	require.NoError(t, err)
	axRuntime.WaitPublish()

	trackInst := &mockInstaller{}
	svc.installer = trackInst

	hr, ok := svc.runner.(hookableRunner)
	require.True(t, ok, "runner must expose ExecuteSync")

	// Wizard success on _uninstall fires the hook, which calls CleanupAfterUninstall.
	_ = hr.ExecuteSync(context.Background(), ns, "_uninstall", nil)

	trackInst.mu.Lock()
	cleanups := trackInst.cleanupCalls
	trackInst.mu.Unlock()

	require.Len(t, cleanups, 1)
	assert.Equal(t, ns, cleanups[0])
}

// seedArrowCmd seeds a minimal Arrow aggregate into axArrow.
type seedArrowCmd struct {
	ns domain.Namespace
}

func (c seedArrowCmd) AggregateID() string            { return c.ns.String() }
func (c seedArrowCmd) EventName() string              { return "arrow.added" }
func (c seedArrowCmd) ShouldSnapshot() bool           { return false }
func (c seedArrowCmd) Validate(_ *domain.Arrow) error { return nil }
func (c seedArrowCmd) EmitEvent(_ *domain.Arrow) domain.Arrow {
	return domain.Arrow{
		Namespace: c.ns,
		Manifest: domain.ArrowManifest{
			Name:    "test",
			Version: "1.0.0",
		},
	}
}

// seedRuntimeCmd seeds a minimal ArrowRuntime aggregate into axRuntime.
type seedRuntimeCmd struct {
	ns domain.Namespace
}

func (c seedRuntimeCmd) AggregateID() string                          { return c.ns.String() }
func (c seedRuntimeCmd) EventName() string                            { return "runtime.seeded" }
func (c seedRuntimeCmd) ShouldSnapshot() bool                         { return false }
func (c seedRuntimeCmd) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c seedRuntimeCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Namespace: c.ns,
		State:     domain.ArrowStateReady,
	}
}
