package projections

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockArrowService implements arrowService for tests.
type mockArrowService struct {
	hasDependentsResult  bool
	hasDependentsErr     error
	cleanupCalled        bool
	beginExecutionCalled bool
	beginExecutionNS     domain.Namespace
}

func (m *mockArrowService) HasDependents(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) (bool, error) {
	return m.hasDependentsResult, m.hasDependentsErr
}

func (m *mockArrowService) CleanupAfterUninstall(
	_ context.Context,
	_ domain.Namespace,
) error {
	m.cleanupCalled = true
	return nil
}

func (m *mockArrowService) BeginExecution(
	_ context.Context,
	ns domain.Namespace,
	_ string,
	_ map[string]string,
) error {
	m.beginExecutionCalled = true
	m.beginExecutionNS = ns
	return nil
}

func (m *mockArrowService) GetWorkDir(_ context.Context, _ domain.Namespace) (string, error) {
	return "/tmp/test-workdir", nil
}

// wizardExecutorFixture holds all dependencies for WizardExecutor tests.
type wizardExecutorFixture struct {
	executor  *WizardExecutor
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	axArrow   asynx.Asynx[domain.Arrow]
	wiz       *mocks.Wizard
	svc       *mockArrowService
}

func newWizardExecutorFixture(t *testing.T) *wizardExecutorFixture {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axRuntime, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(runtimeES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	wiz := &mocks.Wizard{}
	svc := &mockArrowService{}

	executor := NewWizardExecutor(axRuntime, axArrow, wiz)
	executor.SetService(svc)

	return &wizardExecutorFixture{
		executor:  executor,
		axRuntime: axRuntime,
		axArrow:   axArrow,
		wiz:       wiz,
		svc:       svc,
	}
}

// seedRuntime sends a BeginExecution command so the runtime aggregate exists.
func seedRuntime(
	t *testing.T,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	method string,
) {
	t.Helper()
	_, err := axRuntime.Send(context.Background(), arrowcmds.BeginExecution{
		Namespace: ns,
		Method:    method,
	})
	require.NoError(t, err)
	axRuntime.WaitPublish()
}

// makeRuntime constructs an ArrowRuntime with a populated ActiveRun for unit tests
// that call execute() directly (no asynx involved).
func makeRuntime(ns domain.Namespace, method string) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Namespace: ns,
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method:    method,
			Variables: map[string]string{},
		},
	}
}

// --- execute() ---

func TestExecute_NilActiveRun_NoOp(t *testing.T) {
	f := newWizardExecutorFixture(t)

	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		State:     domain.ArrowStateReady,
		ActiveRun: nil,
	}

	// Should not panic and wizard should not be called.
	f.executor.execute(context.Background(), rt)

	assert.False(t, f.wiz.WasExecuteCalled())
}

func TestExecute_WizardSucceeds_SendsEndExecutionSuccess(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = nil

	rt := makeRuntime(ns, "_execute")

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())

	// Verify EndExecution was sent by checking the runtime state via Get.
	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, result.LastReturn.Outcome)
}

func TestExecute_ErrExecutionExists_ReturnsSilently(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = wizard.ErrExecutionExists

	rt := makeRuntime(ns, "_execute")

	// Should not panic and must not call EndExecution (would fail validation if called
	// because the runtime still has an active run).
	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())

	// Runtime still has ActiveRun (EndExecution was NOT sent).
	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.NotNil(t, result.ActiveRun)
}

func TestExecute_WizardFails_SendsEndExecutionFailed(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = errors.New("step failed")

	rt := makeRuntime(ns, "_execute")

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())

	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, result.LastReturn.Outcome)
}

func TestExecute_ExecuteMethodCanceled_BeginStopCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	// Seed arrow with a stop lifecycle so the guard passes.
	_, err := f.axArrow.Send(context.Background(), arrowcmds.AddArrow{
		Namespace: ns,
		Manifest: domain.ArrowManifest{
			Name:    "Repo",
			Version: "1.0.0",
			Lifecycle: domain.Lifecycle{
				Stop: domainstep.StepList{domainstep.NewRunStep("stop", "echo stop", 0, false)},
			},
		},
	})
	require.NoError(t, err)
	f.axArrow.WaitPublish()

	f.wiz.ExecuteErr = context.Canceled

	rt := makeRuntime(ns, "_execute")

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.svc.beginExecutionCalled)
	assert.Equal(t, ns, f.svc.beginExecutionNS)
}

func TestExecute_ExecuteMethodCanceled_NoStopLifecycle_BeginStopNotCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	// Arrow exists but has no stop lifecycle (empty StepList).
	_, err := f.axArrow.Send(context.Background(), arrowcmds.AddArrow{
		Namespace: ns,
		Manifest: domain.ArrowManifest{
			Name:    "Repo",
			Version: "1.0.0",
		},
	})
	require.NoError(t, err)
	f.axArrow.WaitPublish()

	f.wiz.ExecuteErr = context.Canceled

	rt := makeRuntime(ns, "_execute")

	f.executor.execute(context.Background(), rt)

	assert.False(t, f.svc.beginExecutionCalled)
}

func TestExecute_UninstallSuccess_CleanupAfterUninstallCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_uninstall")

	f.wiz.ExecuteErr = nil

	rt := makeRuntime(ns, "_uninstall")

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())
	assert.True(t, f.svc.cleanupCalled)
}

// --- handlePostExecution() ---

func TestHandlePostExecution_ExecuteSuccess_BeginStopNotCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	f.executor.handlePostExecution(
		context.Background(),
		"github.com/org/repo",
		"_execute",
		nil,
		domainRuntime.ExecutionOutcomeSuccess,
	)

	assert.False(t, f.svc.beginExecutionCalled)
}

func TestHandlePostExecution_UninstallFailed_CleanupNotCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	f.executor.handlePostExecution(
		context.Background(),
		"github.com/org/repo",
		"_uninstall",
		errors.New("wizard failed"),
		domainRuntime.ExecutionOutcomeFailed,
	)

	assert.False(t, f.svc.cleanupCalled)
}

func TestHandlePostExecution_UninstallSuccess_CleanupCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	f.executor.handlePostExecution(
		context.Background(),
		"github.com/org/repo",
		"_uninstall",
		nil,
		domainRuntime.ExecutionOutcomeSuccess,
	)

	assert.True(t, f.svc.cleanupCalled)
}

func TestHandlePostExecution_SvcNil_UninstallSuccess_NoPanic(t *testing.T) {
	executor := NewWizardExecutor(nil, nil, nil)
	// svc not set — must not panic.
	assert.NotPanics(t, func() {
		executor.handlePostExecution(
			context.Background(),
			"github.com/org/repo",
			"_uninstall",
			nil,
			domainRuntime.ExecutionOutcomeSuccess,
		)
	})
}

// --- mapOutcome() ---

func TestMapOutcome_Nil_ReturnsSuccess(t *testing.T) {
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, mapOutcome(nil))
}

func TestMapOutcome_Canceled_ReturnsCancelled(t *testing.T) {
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, mapOutcome(context.Canceled))
}

func TestMapOutcome_WrappedCanceled_ReturnsCancelled(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", context.Canceled)
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, mapOutcome(wrapped))
}

func TestMapOutcome_OtherError_ReturnsFailed(t *testing.T) {
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, mapOutcome(errors.New("some error")))
}

// --- Handler() ---

func TestHandler_ReturnsNonNilFunc(t *testing.T) {
	f := newWizardExecutorFixture(t)
	h := f.executor.Handler()
	assert.NotNil(t, h)
}

func TestHandler_InvokesExecute_WithNilActiveRun_NoOp(t *testing.T) {
	f := newWizardExecutorFixture(t)
	h := f.executor.Handler()

	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		ActiveRun: nil,
	}
	evt := asynxModels.Event[domainRuntime.ArrowRuntime]{
		EventName: "runtime.begun",
		Aggregate: rt,
	}

	// Should not panic and wizard must not be called.
	h(context.Background(), evt)
	assert.False(t, f.wiz.WasExecuteCalled())
}

func TestHandler_InvokesExecute_WithActiveRun_CallsWizard(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	h := f.executor.Handler()
	rt := makeRuntime(ns, "_execute")
	evt := asynxModels.Event[domainRuntime.ArrowRuntime]{
		EventName: "runtime.begun",
		Aggregate: rt,
	}

	h(context.Background(), evt)

	assert.True(t, f.wiz.WasExecuteCalled())
}

// --- execute() canceled outcome ---

func TestExecute_WizardCanceled_SendsEndExecutionCancelled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_install")

	f.wiz.ExecuteErr = context.Canceled

	rt := makeRuntime(ns, "_install")

	f.executor.execute(context.Background(), rt)

	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, result.LastReturn.Outcome)
}

func TestExecute_WithSteps_StepsExtracted(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = nil

	step := domainstep.NewRunStep("echo", "echo hello", 0, false)
	rt := domainRuntime.ArrowRuntime{
		Namespace: ns,
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method:    "_execute",
			Variables: map[string]string{},
			Steps: []domainRuntime.StepProgress{
				{Index: 0, Status: domainRuntime.StepStatusPending, Step: step},
			},
		},
	}

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())
}
