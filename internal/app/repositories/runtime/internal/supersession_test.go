package runtimeinternal_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// seedStoppedOverRun reproduces the stop takeover: the arrow is running an
// execution ("run") when a stop begins, replacing it with a stop execution
// ("stop") that keeps the same PID. It returns both execution ids.
func seedStoppedOverRun(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) (runID, stopID string) {
	t.Helper()
	ctx := context.Background()

	_, err := ax.Send(ctx, commands.BeginInstall{Namespace: ns, ExecutionID: "install"})
	require.NoError(t, err)
	_, err = ax.Send(ctx, commands.EndExecution{
		Namespace:   ns,
		ExecutionID: "install",
		Outcome:     domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	_, err = ax.Send(ctx, commands.BeginExecution{
		Namespace:   ns,
		ExecutionID: "run",
		Method:      domain.MethodExecute,
		Steps:       domainStep.StepList{domainStep.NewRunStep("run", "sleep 60", false, "", false)},
	})
	require.NoError(t, err)
	_, err = ax.Send(ctx, commands.AdvanceStep{
		Namespace:   ns,
		ExecutionID: "run",
		StepIndex:   0,
		ToStatus:    domainRuntime.StepStatusRunning,
	})
	require.NoError(t, err)

	_, err = ax.Send(ctx, commands.BeginStop{
		Namespace:   ns,
		ExecutionID: "stop",
		Steps:       domainStep.StepList{domainStep.NewSignalStep("stop", domainStep.SignalKindGraceful, "10s", false)},
	})
	require.NoError(t, err)

	return "run", "stop"
}

func TestDrainExecution_Superseded_LeavesTakeoverExecutionUntouched(t *testing.T) {
	ns := domain.Namespace("github.com/user/superseded@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	runID, _ := seedStoppedOverRun(t, axRuntime, ns)

	// The stop signal kills the running process, so the run it took over ends
	// with a failed step and reports it after the takeover.
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.emit(wizard.Event{Kind: wizard.EventKindStepFailed, StepIndex: 0, Err: assert.AnError})
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), runID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, axRuntime,
	)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution, "the stop execution must still be the current one")
	assert.Equal(t, domain.MethodStop, got.Execution.Method)
	assert.Equal(t, domain.ArrowStateStopping, got.State)
	require.Len(t, got.Execution.Steps, 1)
	assert.Equal(t, domainRuntime.StepStatusPending, got.Execution.Steps[0].Status)
	assert.Nil(t, got.Execution.Steps[0].Error)
}

func TestDrainExecution_Superseded_StopsSendingAfterTakeover(t *testing.T) {
	ns := domain.Namespace("github.com/user/superseded-pid@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	runID, _ := seedStoppedOverRun(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeCancelled)
	exec.emit(wizard.Event{Kind: wizard.EventKindPID, PID: 4242})
	exec.emit(wizard.Event{Kind: wizard.EventKindStepCompleted, StepIndex: 0})
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), runID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, axRuntime,
	)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution)
	assert.Equal(t, 0, got.Execution.PID, "the superseded run must not record its PID on the stop execution")
	assert.Equal(t, domainRuntime.StepStatusPending, got.Execution.Steps[0].Status)
}

func TestDrainExecution_Superseded_DoesNotEndTheTakeoverExecution(t *testing.T) {
	ns := domain.Namespace("github.com/user/superseded-end@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	runID, stopID := seedStoppedOverRun(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeCancelled)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), runID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, axRuntime,
	)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution, "only the stop execution may end itself")
	assert.Equal(t, stopID, got.Execution.ID)
	assert.Equal(t, domain.ArrowStateStopping, got.State)
	require.NotNil(t, got.LastReturn)
	assert.Equal(t, domain.MethodInstall, got.LastReturn.Method, "the superseded run must not publish its own return")
}

func TestDrainExecution_Superseded_TakeoverExecutionStillCompletes(t *testing.T) {
	ns := domain.Namespace("github.com/user/superseded-stop@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	runID, stopID := seedStoppedOverRun(t, axRuntime, ns)

	superseded := newFakeExecution(domainRuntime.ExecutionOutcomeCancelled)
	superseded.emit(wizard.Event{Kind: wizard.EventKindStepFailed, StepIndex: 0, Err: assert.AnError})
	superseded.close()
	runtimeinternal.DrainExecution(
		context.Background(), superseded, ns.String(), runID, domain.MethodExecute, noopMarkInstalled, noopMarkUninstalled, axRuntime,
	)

	stop := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	stop.emit(wizard.Event{Kind: wizard.EventKindStepStarted, StepIndex: 0})
	stop.emit(wizard.Event{Kind: wizard.EventKindStepCompleted, StepIndex: 0})
	stop.close()
	runtimeinternal.DrainExecution(
		context.Background(), stop, ns.String(), stopID, domain.MethodStop, noopMarkInstalled, noopMarkUninstalled, axRuntime,
	)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Execution)
	assert.Equal(t, domain.ArrowStateReady, got.State)
	require.NotNil(t, got.LastReturn)
	assert.Equal(t, domain.MethodStop, got.LastReturn.Method)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, got.LastReturn.Outcome)
	require.Len(t, got.LastReturn.Steps, 1)
	assert.Equal(t, domainRuntime.StepStatusCompleted, got.LastReturn.Steps[0].Status)
	assert.Nil(t, got.LastReturn.Steps[0].Error, "the stop step must not inherit the superseded run's failure")
}
