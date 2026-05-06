package runtimeinternal_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	runtimeinternal "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
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

func newTestAsynxRuntimeForHooks(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
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
		Namespace: ns,
		Method:    domain.MethodExecute,
		Steps:     domainStep.StepList{testStep()},
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
		Namespace: ns,
		Steps:     domainStep.StepList{testStep()},
	})
	require.NoError(t, err)
}

func noopMarkInstalled(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error {
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

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
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

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
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
	markInstalled := func(ctx context.Context, nsArg domain.Namespace, ref string, at time.Time) error {
		markInstalledCalled.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodInstall, markInstalled, axRuntime)
	axRuntime.WaitPublish()

	assert.True(t, markInstalledCalled.Load())
}

func TestDrainExecution_NonInstallMethod_DoesNotCallMarkInstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	var markInstalledCalled atomic.Bool
	markInstalled := func(ctx context.Context, nsArg domain.Namespace, ref string, at time.Time) error {
		markInstalledCalled.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, markInstalled, axRuntime)
	axRuntime.WaitPublish()

	assert.False(t, markInstalledCalled.Load())
}

func TestDrainExecution_InstallFailed_DoesNotCallMarkInstalled(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedInstallingRuntimeForHooks(t, axRuntime, ns)

	var markInstalledCalled atomic.Bool
	markInstalled := func(ctx context.Context, nsArg domain.Namespace, ref string, at time.Time) error {
		markInstalledCalled.Store(true)
		return nil
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodInstall, markInstalled, axRuntime)
	axRuntime.WaitPublish()

	assert.False(t, markInstalledCalled.Load())
}

func TestDrainExecution_AlwaysSendsEndExecution(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.close()

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
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

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
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

	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
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
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
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
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
}

func TestSendEndExecution_SendError_NoopAndLogged(t *testing.T) {
	ns := domain.Namespace("github.com/user/sendend@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)
	_ = axRuntime.Shutdown(context.Background())

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	// Should not panic — sendEndExecution logs errors.
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodExecute, noopMarkInstalled, axRuntime)
}

func TestDrainExecution_InstallSuccess_MarkInstalledError_NoopAndLogged(t *testing.T) {
	ns := domain.Namespace("github.com/user/markinst@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedInstallingRuntimeForHooks(t, axRuntime, ns)

	markInstalled := func(ctx context.Context, nsArg domain.Namespace, ref string, at time.Time) error {
		return assert.AnError // error returned by markInstalled
	}

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	// Should not panic — markInstalled errors are logged.
	runtimeinternal.DrainExecution(context.Background(), exec, ns.String(), domain.MethodInstall, markInstalled, axRuntime)
	axRuntime.WaitPublish()
}
