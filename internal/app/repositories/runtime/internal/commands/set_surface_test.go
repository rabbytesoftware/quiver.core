package commands_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func runningWithSurface(t *testing.T) (asynx.Asynx[domainRuntime.ArrowRuntime], domain.Namespace) {
	t.Helper()
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)
	_, err := ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		ExecutionID: "e1",
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
		Steps:       domainStep.StepList{domainStep.NewRunStep("s", "echo hi", false, "", true)},
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.RecordPID{Namespace: ns, ExecutionID: "e1", PID: 42})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.SetSurface{
		Namespace:   ns,
		ExecutionID: "e1",
		Surface:     domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/", Ready: true},
	})
	require.NoError(t, err)
	return ax, ns
}

func TestSetSurface_SetsSurfaceAndKeepsRest(t *testing.T) {
	ax, ns := runningWithSurface(t)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateRunning, got.State)
	assert.Equal(t, 42, got.Execution.PID)
	assert.Equal(t, &domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/", Ready: true}, got.Execution.Surface)

	cmd := commands.SetSurface{Namespace: ns}
	assert.Equal(t, "runtime.surface_set."+ns.String(), cmd.EventName())
	assert.Equal(t, ns.String(), cmd.AggregateID())
}

func TestSetSurface_DoesNotMutatePrevious(t *testing.T) {
	cur := &domainRuntime.ArrowRuntime{Execution: &domainRuntime.Execution{ID: "e1"}}
	next := commands.SetSurface{ExecutionID: "e1", Surface: domainRuntime.Surface{Path: "/"}}.EmitEvent(cur)
	assert.Nil(t, cur.Execution.Surface)
	require.NotNil(t, next.Execution.Surface)
}

func TestSetSurface_RejectsSupersededExecution(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.SetSurface{Namespace: ns, ExecutionID: "other"})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrExecutionSuperseded)
}

func TestSurface_SurvivesRecordPID(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.RecordPID{Namespace: ns, ExecutionID: "e1", PID: 7})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution.Surface)
	assert.Equal(t, 7, got.Execution.PID)
}

func TestSurface_SurvivesAdvanceStep(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.AdvanceStep{
		Namespace: ns, ExecutionID: "e1", StepIndex: 0, ToStatus: domainRuntime.StepStatusRunning,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, got.Execution.Surface)
	assert.Equal(t, 42, got.Execution.PID)
	assert.Equal(t, domainRuntime.StepStatusRunning, got.Execution.Steps[0].Status)
}

func TestSurface_EndsWithExecution(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns, ExecutionID: "e1", Outcome: domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Execution)
}

func TestSurface_DroppedByRestartExecution(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.RestartExecution{Namespace: ns, ExecutionID: "e1"})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Execution.Surface, "a restarted run has a new process and must reopen its surface")
}

func TestSurface_DroppedByBeginStop(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.BeginStop{Namespace: ns, ExecutionID: "stop-1"})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Execution.Surface)
}

func TestSurface_DroppedByRecordDetached(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.RecordDetached{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Execution)
}

func TestClearSurface_RemovesSurfaceAndKeepsRest(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.ClearSurface{Namespace: ns, ExecutionID: "e1"})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateRunning, got.State)
	assert.Equal(t, 42, got.Execution.PID)
	assert.Nil(t, got.Execution.Surface)
}

func TestClearSurface_DoesNotMutatePrevious(t *testing.T) {
	cur := &domainRuntime.ArrowRuntime{Execution: &domainRuntime.Execution{ID: "e1", Surface: &domainRuntime.Surface{Path: "/"}}}
	next := commands.ClearSurface{ExecutionID: "e1"}.EmitEvent(cur)
	assert.NotNil(t, cur.Execution.Surface)
	assert.Nil(t, next.Execution.Surface)
}

func TestClearSurface_RejectsSupersededExecution(t *testing.T) {
	ax, ns := runningWithSurface(t)
	_, err := ax.Send(context.Background(), commands.ClearSurface{Namespace: ns, ExecutionID: "other"})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrExecutionSuperseded)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.NotNil(t, got.Execution.Surface)
}

func TestClearSurface_NamesItsEvent(t *testing.T) {
	ns := testNs()
	cmd := commands.ClearSurface{Namespace: ns}
	assert.Equal(t, "runtime.surface_cleared."+ns.String(), cmd.EventName())
	assert.Equal(t, ns.String(), cmd.AggregateID())
}
