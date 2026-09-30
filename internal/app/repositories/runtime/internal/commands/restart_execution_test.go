package commands_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestRestartExecution_CurrentExecution_ResetsStepsKeepsRun(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	old := []domainStep.Step{
		domainStep.NewFetchStep("old", "https://x/a", "a", "sha256:aa", "1m", true),
		domainStep.NewRunStep("run", "echo hi", false, "", true),
	}
	_, err := ax.Send(context.Background(), commands.BeginInstall{
		Namespace:   ns,
		ExecutionID: "exec-1",
		Steps:       old,
		Variables:   map[string]string{"K": "v"},
		WorkDir:     "/tmp/w",
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.AdvanceStep{
		Namespace:   ns,
		ExecutionID: "exec-1",
		StepIndex:   0,
		ToStatus:    domainRuntime.StepStatusFailed,
		Error:       new(string),
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.RecordPID{Namespace: ns, ExecutionID: "exec-1", PID: 42})
	require.NoError(t, err)

	fresh := []domainStep.Step{domainStep.NewFetchStep("new", "https://x/a", "a", "sha256:bb", "1m", true)}
	_, err = ax.Send(context.Background(), commands.RestartExecution{
		Namespace:   ns,
		ExecutionID: "exec-1",
		Steps:       fresh,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateInstalling, got.State)
	require.NotNil(t, got.Execution)
	assert.Equal(t, "exec-1", got.Execution.ID)
	assert.Equal(t, domain.MethodInstall, got.Execution.Method)
	assert.Equal(t, "/tmp/w", got.Execution.WorkDir)
	assert.Equal(t, map[string]string{"K": "v"}, got.Execution.Variables)
	assert.Zero(t, got.Execution.PID)
	require.Len(t, got.Execution.Steps, 1)
	assert.Equal(t, domainRuntime.StepStatusPending, got.Execution.Steps[0].Status)
	assert.Nil(t, got.Execution.Steps[0].Error)
	assert.Equal(t, fresh[0], got.Execution.Steps[0].Step)
}

func TestRestartExecution_OtherExecution_IsSuperseded(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns, ExecutionID: "exec-1"})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.RestartExecution{Namespace: ns, ExecutionID: "other"})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrExecutionSuperseded)
}

func TestRestartExecution_NoRuntime_Fails(t *testing.T) {
	ax := buildAsynx(t)

	_, err := ax.Send(context.Background(), commands.RestartExecution{Namespace: testNs(), ExecutionID: "exec-1"})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestRestartExecution_Identity(t *testing.T) {
	cmd := commands.RestartExecution{Namespace: testNs()}

	assert.Equal(t, testNs().String(), cmd.AggregateID())
	assert.Equal(t, "runtime.step_advanced."+testNs().String(), cmd.EventName())
	assert.True(t, cmd.ShouldSnapshot())
}
