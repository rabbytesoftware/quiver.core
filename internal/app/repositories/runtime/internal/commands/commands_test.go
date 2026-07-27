package commands_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func buildAsynx(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
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

func testNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@v1.0.0")
}

func isValidationErr(err error) bool {
	return errors.Is(err, asynxModels.ErrValidation) ||
		errors.Is(err, asynxModels.ErrPipelineFailed)
}

// seedRuntime seeds a runtime in a given state.
func seedRuntime(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	steps []domainStep.Step,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns, Steps: steps})
	require.NoError(t, err)
}

func seedReadyRuntime(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) {
	t.Helper()
	seedRuntime(t, ax, ns, nil)
	// End the install to reach Ready state
	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateReady, got.State)
}

// ─── BeginExecution ──────────────────────────────────────────────────────────

func TestBeginExecution_Execute_SetsRunning(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	cmd := commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateRunning, got.State)
}

func TestBeginExecution_WrongAvailableIn_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	// Try to execute when expecting only Installing state
	cmd := commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateInstalling},
	}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginExecution_AlreadyRunning_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	// Start running
	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	// Try to start again — should fail
	_, err = ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateRunning},
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// ─── BeginInstall ────────────────────────────────────────────────────────────

func TestBeginInstall_OnAbsent_SetsInstalling(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/install-absent@v1")
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateInstalling, got.State)
	require.NotNil(t, got.Execution)
	assert.Equal(t, domain.MethodInstall, got.Execution.Method)
}

func TestBeginInstall_OnAbsentState_SetsInstalling(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/install-absentstate@v1")
	// Failed install → state = Absent (runtime exists with state=Absent)
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{Namespace: ns, Outcome: domainRuntime.ExecutionOutcomeFailed})
	require.NoError(t, err)
	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateAbsent, got.State)
	// Re-install from Absent state
	_, err = ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	got, err = ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateInstalling, got.State)
}

func TestBeginInstall_OnReady_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/install-ready@v1")
	seedReadyRuntime(t, ax, ns)
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginInstall_WithSteps_SetsSteps(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/install-steps@v1")
	step := domainStep.NewRunStep("install step", "echo installed", false, "", true)
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns, Steps: domainStep.StepList{step}})
	require.NoError(t, err)
	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Len(t, got.Execution.Steps, 1)
	assert.Equal(t, domainRuntime.StepStatusPending, got.Execution.Steps[0].Status)
}

func TestBeginInstall_OnRemoved_SetsInstalling(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/install-removed@v1")

	// Seed directly into Removed state
	_, err := ax.Send(context.Background(), forceRemovedCmd{ns: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateRemoved, got.State)

	// Install from Removed — should succeed
	_, err = ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)

	got, err = ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateInstalling, got.State)
}

type forceRemovedCmd struct {
	ns domain.Namespace
}

func (c forceRemovedCmd) AggregateID() string  { return c.ns.String() }
func (c forceRemovedCmd) EventName() string    { return "runtime.removed." + c.ns.String() }
func (c forceRemovedCmd) ShouldSnapshot() bool { return true }
func (c forceRemovedCmd) Validate(_ *domainRuntime.ArrowRuntime) error {
	return nil
}

func (c forceRemovedCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.ns,
		State: domain.ArrowStateRemoved,
	}
}

// ─── EndExecution ────────────────────────────────────────────────────────────

func TestEndExecution_AfterInstall_Success_SetsReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedRuntime(t, ax, ns, nil)

	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
	assert.Nil(t, got.Execution)
}

func TestEndExecution_AfterInstall_Failure_SetsAbsent(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedRuntime(t, ax, ns, nil)

	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeFailed,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestEndExecution_AfterUninstall_Success_SetsAbsent(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestEndExecution_WithoutExecution_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// ─── RecoverInterrupted ──────────────────────────────────────────────────────

func TestRecoverInterrupted_Installing_SetsAbsent(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedRuntime(t, ax, ns, nil)

	_, err := ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestRecoverInterrupted_Uninstalling_SetsAbsent(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestRecoverInterrupted_Updating_SetsAbsent(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/recover-updating@v1.0.0")
	seedOutdatedRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUpdate{Namespace: ns})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestRecoverInterrupted_Running_SetsReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRecoverInterrupted_Stopping_SetsReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/recover-stopping@v1.0.0")
	seedReadyRuntime(t, ax, ns)

	// Execute first
	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	// Stop via dedicated command
	_, err = ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRecoverInterrupted_Draining_SetsReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	depNs := domain.Namespace("github.com/user/dep@v1.0.0")

	// Seed directly into Draining state.
	_, err := ax.Send(context.Background(), forceDrainingCmd{
		ns:    ns,
		depNs: depNs,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateDraining, got.State)

	_, err = ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.NoError(t, err)

	got, err = ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

type forceDrainingCmd struct {
	ns    domain.Namespace
	depNs domain.Namespace
}

func (c forceDrainingCmd) AggregateID() string  { return c.ns.String() }
func (c forceDrainingCmd) EventName() string    { return "runtime.draining." + c.ns.String() }
func (c forceDrainingCmd) ShouldSnapshot() bool { return true }
func (c forceDrainingCmd) Validate(_ *domainRuntime.ArrowRuntime) error {
	return nil
}

func (c forceDrainingCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.ns,
		State: domain.ArrowStateDraining,
	}
}

func TestRecoverInterrupted_StableState_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	// Ready is stable, should fail
	_, err := ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestRecoverInterrupted_NoRuntime_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	_, err := ax.SendWait(context.Background(), commands.RecoverInterrupted{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// ─── AdvanceStep ─────────────────────────────────────────────────────────────

func TestAdvanceStep_ValidRuntime_UpdatesStep(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	step0 := domainStep.NewRunStep("step 0", "echo hi", false, "", true)
	seedRuntime(t, ax, ns, domainStep.StepList{step0})

	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusRunning,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Len(t, got.Execution.Steps, 1)
	assert.Equal(t, domainRuntime.StepStatusRunning, got.Execution.Steps[0].Status)
}

func TestAdvanceStep_WithError_SetsErrorField(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	step0 := domainStep.NewRunStep("step 0", "exit 1", false, "", true)
	seedRuntime(t, ax, ns, domainStep.StepList{step0})

	errStr := "step failed: exit code 1"
	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusFailed,
		Error:     &errStr,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution.Steps[0].Error)
	assert.Equal(t, errStr, *got.Execution.Steps[0].Error)
}

func TestAdvanceStep_PreservesWorkDir(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	step0 := domainStep.NewRunStep("step 0", "echo hi", false, "", true)
	_, err := ax.Send(context.Background(), commands.BeginInstall{
		Namespace: ns,
		Steps:     domainStep.StepList{step0},
		WorkDir:   "/tmp/workdir",
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusRunning,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "/tmp/workdir", got.Execution.WorkDir, "WorkDir must survive AdvanceStep")
}

func TestAdvanceStep_NoExecution_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusRunning,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestAdvanceStep_WritesSnapshot(t *testing.T) {
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

	ns := testNs()
	step0 := domainStep.NewRunStep("step 0", "echo hi", false, "", true)
	seedRuntime(t, ax, ns, domainStep.StepList{step0})

	// BeginInstall already snapshots; clear it so a found snapshot below can
	// only have come from AdvanceStep itself.
	require.NoError(t, ss.Delete(context.Background(), ns.String()))

	_, err = ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusRunning,
	})
	require.NoError(t, err)

	_, found, err := ss.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, found, "advance step must persist a snapshot; without one every read cold-replays the runtime's full history forever")
}

// ─── RecordDetached ──────────────────────────────────────────────────────────

func TestRecordDetached_FromRunning_SetsDetached(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), commands.RecordDetached{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateDetached, got.State)
	assert.Nil(t, got.Execution)
}

func TestRecordDetached_NotFromRunning_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	// Ready cannot transition to Detached
	_, err := ax.SendWait(context.Background(), commands.RecordDetached{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// ─── RecordPID ───────────────────────────────────────────────────────────────

func TestRecordPID_WhileRunning_SetsPID(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.RecordPID{
		Namespace: ns,
		PID:       12345,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution)
	assert.Equal(t, 12345, got.Execution.PID)
}

func TestRecordPID_NoActiveExecution_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.RecordPID{
		Namespace: ns,
		PID:       12345,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestRecordPID_WhileInstalling_SetsPID(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedRuntime(t, ax, ns, nil)

	_, err := ax.Send(context.Background(), commands.RecordPID{
		Namespace: ns,
		PID:       99999,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution)
	assert.Equal(t, 99999, got.Execution.PID)
}

// ─── Additional Validate branches ────────────────────────────────────────────

func TestEndExecution_stateAfterEnd_ExecuteSuccess_SetsReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/exsuccess@v1.0.0")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestEndExecution_stateAfterEnd_ExecuteFailed_SetsReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/exfail@v1.0.0")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeFailed,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestEndExecution_stateAfterEnd_UninstallFailed_SetsReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/uninstfail@v1.0.0")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeFailed,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestAdvanceStep_NilCurrent_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/adv@v1.0.0")

	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusRunning,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestAdvanceStep_ReadyRuntime_NoExecution_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/advnoexec@v1.0.0")
	// Seed a ready runtime (no active execution)
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns,
		StepIndex: 0,
		ToStatus:  domainRuntime.StepStatusRunning,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestEndExecution_ReadyRuntime_NoExecution_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/endnoexec@v1.0.0")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestRecordDetached_NilCurrent_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/detachnil@v1.0.0")

	_, err := ax.SendWait(context.Background(), commands.RecordDetached{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestRecordPID_NilCurrent_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/pidnil@v1.0.0")

	_, err := ax.Send(context.Background(), commands.RecordPID{
		Namespace: ns,
		PID:       999,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// ─── MarkOutdated ────────────────────────────────────────────────────────────

func TestMarkOutdated_FromReady_SetsOutdated(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	added := []domain.Namespace{"github.com/user/new-dep@v1"}
	removed := []domain.Namespace{"github.com/user/old-dep@v1"}

	_, err := ax.Send(context.Background(), commands.MarkOutdated{
		Namespace:   ns,
		AddedDeps:   added,
		RemovedDeps: removed,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
	require.NotNil(t, got.PendingDepSync)
	assert.Equal(t, added, got.PendingDepSync.AddedDeps)
	assert.Equal(t, removed, got.PendingDepSync.RemovedDeps)
}

func TestMarkOutdated_NilCurrent_SetsOutdated(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/outdatednil@v1.0.0")

	added := []domain.Namespace{"github.com/user/new-dep@v1"}
	_, err := ax.Send(context.Background(), commands.MarkOutdated{
		Namespace: ns,
		AddedDeps: added,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
	require.NotNil(t, got.PendingDepSync)
	assert.Equal(t, added, got.PendingDepSync.AddedDeps)
}

func TestMarkOutdated_NonReady_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedRuntime(t, ax, ns, nil)

	_, err := ax.Send(context.Background(), commands.MarkOutdated{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// ─── BeginUninstall ──────────────────────────────────────────────────────────

func TestBeginUninstall_FromReady_SetsUninstalling(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/uninstall-ready@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateUninstalling, got.State)
	require.NotNil(t, got.Execution)
	assert.Equal(t, domain.MethodUninstall, got.Execution.Method)
}

func TestBeginUninstall_NotFromReady_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/uninstall-absent@v1")

	_, err := ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginUninstall_AlreadyExecuting_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/uninstall-executing@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.NoError(t, err)

	// Second uninstall while first is in progress
	_, err = ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginUninstall_PreservesLastReturn(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/uninstall-lastreturn@v1")
	seedReadyRuntime(t, ax, ns)

	// Seed a LastReturn via a custom execution
	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns, Method: "_execute",
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns, Outcome: domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.LastReturn)

	_, err = ax.Send(context.Background(), commands.BeginUninstall{Namespace: ns})
	require.NoError(t, err)

	got, err = ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.NotNil(t, got.LastReturn, "LastReturn must be preserved across BeginUninstall")
}

// ─── BeginStop ───────────────────────────────────────────────────────────────

func TestBeginStop_FromRunning_SetsStopping(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/stop-running@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns, Method: domain.MethodExecute,
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateStopping, got.State)
	require.NotNil(t, got.Execution)
	assert.Equal(t, domain.MethodStop, got.Execution.Method)
}

func TestBeginStop_CarriesPIDFromRunning(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/stop-pid@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns, Method: domain.MethodExecute,
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.RecordPID{Namespace: ns, PID: 42})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution)
	assert.Equal(t, 42, got.Execution.PID, "PID must be carried into stop execution")
}

func TestBeginStop_FromDetached_SetsStopping(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/stop-detached@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns, Method: domain.MethodExecute,
	})
	require.NoError(t, err)
	_, err = ax.SendWait(context.Background(), commands.RecordDetached{Namespace: ns})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateStopping, got.State)
}

func TestBeginStop_NotFromRunning_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/stop-ready@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginStop_AlreadyStopping_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/stop-stopping@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns, Method: domain.MethodExecute,
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.NoError(t, err)

	// Double-stop must fail
	_, err = ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// ─── BeginUpdate ─────────────────────────────────────────────────────────────

func seedOutdatedRuntime(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) {
	t.Helper()
	seedReadyRuntime(t, ax, ns)
	_, err := ax.Send(context.Background(), commands.MarkOutdated{Namespace: ns})
	require.NoError(t, err)
	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateOutdated, got.State)
}

func TestBeginUpdate_FromOutdated_SetsUpdating(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/update-outdated@v1")
	seedOutdatedRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUpdate{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateUpdating, got.State)
	require.NotNil(t, got.Execution)
	assert.Equal(t, domain.MethodUpdate, got.Execution.Method)
	assert.Nil(t, got.PendingDepSync, "PendingDepSync must be cleared on BeginUpdate")
}

func TestBeginUpdate_WithZeroSteps_SetsUpdating(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/update-zerosteps@v1")
	seedOutdatedRuntime(t, ax, ns)

	// Zero steps is fine — wizard runs nothing and EndExecution fires
	_, err := ax.Send(context.Background(), commands.BeginUpdate{Namespace: ns, Steps: nil})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateUpdating, got.State)
	assert.Len(t, got.Execution.Steps, 0)
}

func TestBeginUpdate_FromReady_SetsUpdating(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/update-fromready@v1")
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), commands.BeginUpdate{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateUpdating, got.State)
	require.NotNil(t, got.Execution)
	assert.Equal(t, domain.MethodUpdate, got.Execution.Method)
}

func TestBeginUpdate_FromRunning_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/update-running@v1")
	seedReadyRuntime(t, ax, ns)
	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.BeginUpdate{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginUpdate_OnAbsent_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/update-absent@v1")

	_, err := ax.Send(context.Background(), commands.BeginUpdate{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginUpdate_PreservesLastReturn(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/update-lastreturn@v1")
	seedReadyRuntime(t, ax, ns)

	// Seed LastReturn
	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns, Method: "_execute",
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns, Outcome: domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	// Mark outdated then update
	_, err = ax.Send(context.Background(), commands.MarkOutdated{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.BeginUpdate{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.NotNil(t, got.LastReturn, "LastReturn must survive BeginUpdate")
}

// ─── Execution identity ──────────────────────────────────────────────────────

// seedRunningWithID seeds an arrow running executionID with a single step.
func seedRunningWithID(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	executionID string,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), commands.BeginInstall{
		Namespace:   ns,
		ExecutionID: executionID,
		Steps:       domainStep.StepList{domainStep.NewRunStep("step 0", "echo hi", false, "", true)},
	})
	require.NoError(t, err)
}

func TestAdvanceStep_ForeignExecution_IsSuperseded(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/advance-foreign@v1")
	seedRunningWithID(t, ax, ns, "current")

	errStr := "killed"
	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace:   ns,
		ExecutionID: "gone",
		StepIndex:   0,
		ToStatus:    domainRuntime.StepStatusFailed,
		Error:       &errStr,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrExecutionSuperseded)
	assert.True(t, isValidationErr(err))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusPending, got.Execution.Steps[0].Status)
	assert.Nil(t, got.Execution.Steps[0].Error)
}

func TestAdvanceStep_CurrentExecution_KeepsTheID(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/advance-keeps-id@v1")
	seedRunningWithID(t, ax, ns, "current")

	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace:   ns,
		ExecutionID: "current",
		StepIndex:   0,
		ToStatus:    domainRuntime.StepStatusRunning,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "current", got.Execution.ID, "the execution id must survive AdvanceStep")
}

func TestAdvanceStep_IndexOutsideTheExecution_Fails(t *testing.T) {
	testCases := []struct {
		name  string
		index int
	}{
		{name: "past the last step", index: 1},
		{name: "negative", index: -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := buildAsynx(t)
			ns := domain.Namespace("github.com/user/advance-bounds@v1")
			seedRunningWithID(t, ax, ns, "current")

			_, err := ax.Send(context.Background(), commands.AdvanceStep{
				Namespace:   ns,
				ExecutionID: "current",
				StepIndex:   tc.index,
				ToStatus:    domainRuntime.StepStatusRunning,
			})
			require.Error(t, err)
			assert.True(t, isValidationErr(err))
		})
	}
}

func TestRecordPID_ForeignExecution_IsSuperseded(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/pid-foreign@v1")
	seedRunningWithID(t, ax, ns, "current")

	_, err := ax.Send(context.Background(), commands.RecordPID{
		Namespace:   ns,
		ExecutionID: "gone",
		PID:         4242,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrExecutionSuperseded)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, 0, got.Execution.PID)
}

func TestEndExecution_ForeignExecution_IsSuperseded(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/end-foreign@v1")
	seedRunningWithID(t, ax, ns, "current")

	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace:   ns,
		ExecutionID: "gone",
		Outcome:     domainRuntime.ExecutionOutcomeSuccess,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrExecutionSuperseded)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution, "a foreign end must not clear the current execution")
	assert.Equal(t, domain.ArrowStateInstalling, got.State)
}

func TestEndExecution_NoRuntime_IsNotSuperseded(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/end-missing@v1")

	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace:   ns,
		ExecutionID: "current",
		Outcome:     domainRuntime.ExecutionOutcomeSuccess,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
	assert.NotErrorIs(t, err, apperrors.ErrExecutionSuperseded)
}

func TestBeginCommands_CarryTheExecutionID(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/begin-ids@v1")

	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns, ExecutionID: "install"})
	require.NoError(t, err)
	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "install", got.Execution.ID)

	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace:   ns,
		ExecutionID: "install",
		Outcome:     domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		ExecutionID: "run",
		Method:      domain.MethodExecute,
	})
	require.NoError(t, err)
	got, err = ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "run", got.Execution.ID)

	_, err = ax.Send(context.Background(), commands.BeginStop{Namespace: ns, ExecutionID: "stop"})
	require.NoError(t, err)
	got, err = ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "stop", got.Execution.ID)
}

func TestBeginUninstallAndUpdate_CarryTheExecutionID(t *testing.T) {
	ax := buildAsynx(t)
	nsUninstall := domain.Namespace("github.com/user/begin-uninstall-id@v1")
	seedReadyRuntime(t, ax, nsUninstall)

	_, err := ax.Send(context.Background(), commands.BeginUninstall{
		Namespace:   nsUninstall,
		ExecutionID: "uninstall",
	})
	require.NoError(t, err)
	got, err := ax.Get(context.Background(), nsUninstall.String())
	require.NoError(t, err)
	assert.Equal(t, "uninstall", got.Execution.ID)

	nsUpdate := domain.Namespace("github.com/user/begin-update-id@v1")
	seedReadyRuntime(t, ax, nsUpdate)

	_, err = ax.Send(context.Background(), commands.BeginUpdate{
		Namespace:   nsUpdate,
		ExecutionID: "update",
	})
	require.NoError(t, err)
	got, err = ax.Get(context.Background(), nsUpdate.String())
	require.NoError(t, err)
	assert.Equal(t, "update", got.Execution.ID)
}

// ─── Begin* guards ───────────────────────────────────────────────────────────

func TestBeginExecution_NoRuntime_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/exec-missing@v1")

	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns,
		Method:    domain.MethodExecute,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginExecution_NoAvailableIn_RequiresReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/exec-not-ready@v1")
	seedRuntime(t, ax, ns, nil)
	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeFailed,
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns,
		Method:    domain.MethodExecute,
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginExecution_FirstEverExecution_HasNoLastReturn(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/exec-first@v1")

	// A brand-new aggregate: BeginExecution is rejected, so drive EmitEvent
	// through the only command that accepts a nil current — BeginInstall — and
	// then execute, proving preserveLastReturn survives an empty history.
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.LastReturn)
}

func TestBeginInstall_WhileExecuting_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/install-busy@v1")
	seedRuntime(t, ax, ns, nil)

	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginStop_NoRuntime_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/stop-missing@v1")

	_, err := ax.Send(context.Background(), commands.BeginStop{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestBeginExecution_NilCurrent_EmitsWithoutLastReturn(t *testing.T) {
	got := commands.BeginExecution{
		Namespace: testNs(),
		Method:    domain.MethodExecute,
	}.EmitEvent(nil)

	assert.Nil(t, got.LastReturn)
	assert.Equal(t, domain.ArrowStateRunning, got.State)
}

func TestBeginUninstall_ReadyWithExecution_Fails(t *testing.T) {
	err := commands.BeginUninstall{Namespace: testNs()}.Validate(&domainRuntime.ArrowRuntime{
		Ref:       testNs(),
		State:     domain.ArrowStateReady,
		Execution: &domainRuntime.Execution{ID: "busy", Method: domain.MethodExecute},
	})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// AdvanceStep, RecordPID and EndExecution each rebuild ArrowRuntime field by
// field rather than copying it, so any field they forget is silently dropped on
// the next event. PendingDepSync is only ever set while an arrow is outdated,
// which today is a state with no live execution — so nothing reaches these
// commands with one set, and a regression here would stay invisible until some
// future command made that combination reachable. Pin it now instead.
func TestCommands_EmitEvent_PreservePendingDepSync(t *testing.T) {
	const ns = domain.Namespace("github.com/org/app@v1")

	pending := &domainRuntime.DepSyncInfo{
		AddedDeps:   []domain.Namespace{"github.com/org/added@v1"},
		RemovedDeps: []domain.Namespace{"github.com/org/removed@v1"},
	}

	newCurrent := func() *domainRuntime.ArrowRuntime {
		return &domainRuntime.ArrowRuntime{
			Ref:   ns,
			State: domain.ArrowStateInstalling,
			Execution: &domainRuntime.Execution{
				ID:     "exec-1",
				Method: domain.MethodInstall,
				Steps:  []domainRuntime.StepProgress{{Status: domainRuntime.StepStatusRunning}},
			},
			PendingDepSync: pending,
		}
	}

	testCases := []struct {
		name string
		cmd  interface {
			EmitEvent(*domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime
		}
	}{
		{
			name: "AdvanceStep",
			cmd: commands.AdvanceStep{
				Namespace:   ns,
				ExecutionID: "exec-1",
				StepIndex:   0,
				ToStatus:    domainRuntime.StepStatusCompleted,
			},
		},
		{
			name: "RecordPID",
			cmd:  commands.RecordPID{Namespace: ns, ExecutionID: "exec-1", PID: 4242},
		},
		{
			name: "EndExecution",
			cmd: commands.EndExecution{
				Namespace:   ns,
				ExecutionID: "exec-1",
				Outcome:     domainRuntime.ExecutionOutcomeSuccess,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cmd.EmitEvent(newCurrent())
			assert.Equal(t, pending, got.PendingDepSync,
				"%s must carry PendingDepSync through rather than dropping it", tc.name)
		})
	}
}
