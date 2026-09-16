package runtimeinternal_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// ─── fakeExecution ────────────────────────────────────────────────────────────

type fakeExecution struct {
	events  chan wizard.Event
	done    chan struct{}
	outcome domainRuntime.ExecutionOutcome
}

func newFakeExecution(outcome domainRuntime.ExecutionOutcome) *fakeExecution {
	return &fakeExecution{
		events:  make(chan wizard.Event, 16),
		done:    make(chan struct{}),
		outcome: outcome,
	}
}

func (e *fakeExecution) Events() <-chan wizard.Event             { return e.events }
func (e *fakeExecution) Done() <-chan struct{}                   { return e.done }
func (e *fakeExecution) Outcome() domainRuntime.ExecutionOutcome { return e.outcome }

func (e *fakeExecution) emit(evt wizard.Event) {
	e.events <- evt
}

func (e *fakeExecution) close() {
	close(e.events)
	close(e.done)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// testExecutionID is the execution the seeded runtimes are running, so a drain
// stamped with it is the aggregate's current one.
const testExecutionID = "exec-1"

func newTestAsynxRuntimeForHooks(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func testStep() domainStep.Step {
	return domainStep.NewRunStep("test step", "echo hi", false, "", true)
}

func seedRunningRuntimeForHooks(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		ExecutionID: testExecutionID,
		Method:      domain.MethodExecute,
		Steps:       domainStep.StepList{testStep()},
	})
	require.NoError(t, err)
}

func seedInstallingRuntimeForHooks(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), commands.BeginInstall{
		Namespace:   ns,
		ExecutionID: testExecutionID,
		Steps:       domainStep.StepList{testStep()},
	})
	require.NoError(t, err)
}

// seedUninstallingRuntimeForHooks brings a runtime up to `ready` and then puts
// it into an _uninstall execution, which is the only state the uninstall stamp
// can be written from.
func seedUninstallingRuntimeForHooks(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.BeginUninstall{
		Namespace:   ns,
		ExecutionID: testExecutionID,
		Steps:       domainStep.StepList{testStep()},
	})
	require.NoError(t, err)
}

func noopMarkInstalled(ctx context.Context, ns domain.Namespace, at time.Time) error {
	return nil
}

func noopMarkUninstalled(ctx context.Context, ns domain.Namespace) error {
	return nil
}

func noopMarkLastUsed(ctx context.Context, ns domain.Namespace, at time.Time) error {
	return nil
}

// ─── DrainExecution tests ─────────────────────────────────────────────────────

func TestDrainExecution_StepStarted_SendsAdvanceStepRunning(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)

	// Need an executing runtime for AdvanceStep to work
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindStepStarted, StepIndex: 0})
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	// EndExecution is called at end, so state transitions to Ready
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestDrainExecution_PIDEvent_SendsRecordPID(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)

	seedRunningRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindPID, PID: 42})
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	// EndExecution clears execution, so state is Ready after success
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestDrainExecution_InstallSuccess_CallsMarkInstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedInstallingRuntimeForHooks(t, axRuntime, ns)

	var markInstalledCalled atomic.Bool
	markInstalled := func(ctx context.Context, nsArg domain.Namespace, at time.Time) error {
		markInstalledCalled.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodInstall, markInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	assert.True(t, markInstalledCalled.Load())
}

// The stamp reaches the right catalog row only if the hook forwards the whole
// namespace@ref it ran for. A bare namespace, or one carrying another ref, would
// stamp a version nobody installed — so both ref shapes are pinned here.
func TestDrainExecution_InstallSuccess_StampsTheFullNamespace(t *testing.T) {
	testCases := []struct {
		name string
		ns   domain.Namespace
	}{
		{
			name: "tag",
			ns:   domain.Namespace("github.com/user/repo@v1.0.0"),
		},
		{
			name: "default branch",
			ns:   domain.Namespace("github.com/user/repo@master"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			axRuntime := newTestAsynxRuntimeForHooks(t)
			seedInstallingRuntimeForHooks(t, axRuntime, tc.ns)

			var mu sync.Mutex
			var gotNs domain.Namespace
			var gotAt time.Time
			markInstalled := func(_ context.Context, ns domain.Namespace, at time.Time) error {
				mu.Lock()
				defer mu.Unlock()
				gotNs = ns
				gotAt = at
				return nil
			}

			exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
			exec.close()

			runtimeinternal.DrainExecution(
				context.Background(), exec, tc.ns.String(), testExecutionID, domain.MethodInstall, markInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
			)
			axRuntime.WaitPublish()

			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, tc.ns, gotNs)
			assert.False(t, gotAt.IsZero())
		})
	}
}

func TestDrainExecution_NonInstallMethod_DoesNotCallMarkInstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	var markInstalledCalled atomic.Bool
	markInstalled := func(ctx context.Context, nsArg domain.Namespace, at time.Time) error {
		markInstalledCalled.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, markInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	assert.False(t, markInstalledCalled.Load())
}

func TestDrainExecution_InstallFailed_DoesNotCallMarkInstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedInstallingRuntimeForHooks(t, axRuntime, ns)

	var markInstalledCalled atomic.Bool
	markInstalled := func(ctx context.Context, nsArg domain.Namespace, at time.Time) error {
		markInstalledCalled.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodInstall, markInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	assert.False(t, markInstalledCalled.Load())
}

func TestDrainExecution_AlwaysSendsEndExecution(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	// After failed execute, state is Ready
	assert.Equal(t, domain.ArrowStateReady, got.State)
	assert.NotNil(t, got.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, got.LastReturn.Outcome)
}

func TestDrainExecution_StepCompleted_SendsAdvanceStepCompleted(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindStepCompleted, StepIndex: 0})
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	// No panic and endExecution was sent
	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestDrainExecution_StepFailed_SendsAdvanceStepFailed(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	testErr := assert.AnError
	exec.emit(wizard.Event{Kind: wizard.EventKindStepFailed, StepIndex: 0, Err: testErr})
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

// sendStep, sendPID, sendEndExecution all log errors and don't return them.
// We cover the "no-op" (error logged) paths by sending to a shut-down asynx.

func TestSendStep_SendError_NoopAndLogged(t *testing.T) {
	// Create and immediately shut down the runtime so Send fails.
	ns := domain.Namespace("github.com/user/sendstep@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	// First seed a running runtime before we shut down.
	seedRunningRuntimeForHooks(t, axRuntime, ns)
	_ = axRuntime.Shutdown(context.Background())

	// DrainExecution will call sendStep (which calls axRuntime.Send on shut-down runtime).
	// It should not panic — errors are only logged.
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindStepStarted, StepIndex: 0})
	exec.close()

	// Should not panic.
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
}

func TestSendPID_SendError_NoopAndLogged(t *testing.T) {
	ns := domain.Namespace("github.com/user/sendpid@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)
	_ = axRuntime.Shutdown(context.Background())

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindPID, PID: 42})
	exec.close()

	// Should not panic.
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
}

func TestSendEndExecution_SendError_NoopAndLogged(t *testing.T) {
	ns := domain.Namespace("github.com/user/sendend@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)
	_ = axRuntime.Shutdown(context.Background())

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	// Should not panic — sendEndExecution logs errors.
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
}

func TestDrainExecution_InstallSuccess_MarkInstalledError_NoopAndLogged(t *testing.T) {
	ns := domain.Namespace("github.com/user/markinst@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedInstallingRuntimeForHooks(t, axRuntime, ns)

	markInstalled := func(ctx context.Context, nsArg domain.Namespace, at time.Time) error {
		return assert.AnError // error returned by markInstalled
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	// Should not panic — markInstalled errors are logged.
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), testExecutionID, domain.MethodInstall, markInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime)
	axRuntime.WaitPublish()
}

func TestDrainExecution_UninstallSuccess_CallsMarkUninstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedUninstallingRuntimeForHooks(t, axRuntime, ns)

	var mu sync.Mutex
	var gotNs domain.Namespace
	var calls int
	markUninstalled := func(_ context.Context, nsArg domain.Namespace) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		gotNs = nsArg
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodUninstall,
		noopMarkInstalled, markUninstalled, noopMarkLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, calls, "a successful uninstall clears the installed ref exactly once")
	assert.Equal(t, ns, gotNs)
}

func TestDrainExecution_UninstallFailed_DoesNotCallMarkUninstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedUninstallingRuntimeForHooks(t, axRuntime, ns)

	var called atomic.Bool
	markUninstalled := func(_ context.Context, _ domain.Namespace) error {
		called.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodUninstall,
		noopMarkInstalled, markUninstalled, noopMarkLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	assert.False(t, called.Load(), "a failed uninstall leaves the arrow installed")
}

func TestDrainExecution_NonUninstallMethod_DoesNotCallMarkUninstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	var called atomic.Bool
	markUninstalled := func(_ context.Context, _ domain.Namespace) error {
		called.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, markUninstalled, noopMarkLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	assert.False(t, called.Load())
}

func TestDrainExecution_UninstallSuccess_MarkUninstalledError_NoopAndLogged(t *testing.T) {
	ns := domain.Namespace("github.com/user/markuninst@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedUninstallingRuntimeForHooks(t, axRuntime, ns)

	markUninstalled := func(_ context.Context, _ domain.Namespace) error {
		return assert.AnError
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	// Should not panic — markUninstalled errors are logged, and the runtime
	// transition still commits.
	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodUninstall,
		noopMarkInstalled, markUninstalled, noopMarkLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestDrainExecution_ExecuteSuccess_CallsMarkLastUsed(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	var mu sync.Mutex
	var gotNs domain.Namespace
	var gotAt time.Time
	markLastUsed := func(_ context.Context, ns domain.Namespace, at time.Time) error {
		mu.Lock()
		defer mu.Unlock()
		gotNs = ns
		gotAt = at
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, markLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, ns, gotNs)
	assert.False(t, gotAt.IsZero())
}

func TestDrainExecution_NonExecuteMethod_DoesNotCallMarkLastUsed(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedInstallingRuntimeForHooks(t, axRuntime, ns)

	var called atomic.Bool
	markLastUsed := func(_ context.Context, _ domain.Namespace, _ time.Time) error {
		called.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodInstall,
		noopMarkInstalled, noopMarkUninstalled, markLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	assert.False(t, called.Load())
}

func TestDrainExecution_ExecuteFailed_DoesNotCallMarkLastUsed(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	var called atomic.Bool
	markLastUsed := func(_ context.Context, _ domain.Namespace, _ time.Time) error {
		called.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, markLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	assert.False(t, called.Load(), "a failed run must not stamp last-used")
}

func TestDrainExecution_ExecuteSuccess_MarkLastUsedError_NoopAndLogged(t *testing.T) {
	ns := domain.Namespace("github.com/user/marklastused@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	markLastUsed := func(_ context.Context, _ domain.Namespace, _ time.Time) error {
		return assert.AnError
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	// Should not panic — markLastUsed errors are logged, and the runtime
	// transition still commits.
	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, markLastUsed, axRuntime,
	)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}
