package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeginExecution_AggregateID(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestBeginExecution_EventName(t *testing.T) {
	assert.Equal(t, "runtime.begun", BeginExecution{}.EventName())
}

func TestBeginExecution_Validate_NilRuntime_NoAvailableIn_ReturnsNil(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo"}
	require.NoError(t, cmd.Validate(nil))
}

func TestBeginExecution_Validate_AlreadyRunning_ReturnsError(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{
		Ref:       "github.com/org/repo",
		State:     domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{Method: "_execute"},
	}
	err := cmd.Validate(rt)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestBeginExecution_EmitEvent_SetsExecutingState(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_execute"}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.ArrowStateExecuting, result.State)
	require.NotNil(t, result.Execution)
	assert.Equal(t, "_execute", result.Execution.Method)
}

func TestBeginExecution_EmitEvent_InstallMethod_SetsInstalling(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_install"}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.ArrowStateInstalling, result.State)
}

func TestBeginExecutionCmd_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}

func TestBeginExecution_Validate_StopMethod_RequiresRunning(t *testing.T) {
	cmd := BeginExecution{
		Namespace:   "github.com/org/repo",
		Method:      "_stop",
		AvailableIn: []domain.ArrowState{domain.ArrowStateRunning},
	}
	rtRunning := &domainRuntime.ArrowRuntime{
		Ref:   "github.com/org/repo",
		State: domain.ArrowStateRunning,
	}
	require.NoError(t, cmd.Validate(rtRunning))

	rtReady := &domainRuntime.ArrowRuntime{
		Ref:   "github.com/org/repo",
		State: domain.ArrowStateReady,
	}
	err := cmd.Validate(rtReady)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestBeginExecution_Validate_UninstallMethod_RequiresReady(t *testing.T) {
	cmd := BeginExecution{
		Namespace:   "github.com/org/repo",
		Method:      "_uninstall",
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	}
	rtReady := &domainRuntime.ArrowRuntime{
		Ref:   "github.com/org/repo",
		State: domain.ArrowStateReady,
	}
	require.NoError(t, cmd.Validate(rtReady))

	rtAbsent := &domainRuntime.ArrowRuntime{
		Ref:   "github.com/org/repo",
		State: domain.ArrowStateAbsent,
	}
	err := cmd.Validate(rtAbsent)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestBeginExecution_Validate_AbsentWithAvailableInAbsent_ReturnsNil(t *testing.T) {
	cmd := BeginExecution{
		Namespace:   "github.com/org/repo",
		Method:      "_install",
		AvailableIn: []domain.ArrowState{domain.ArrowStateAbsent},
	}
	require.NoError(t, cmd.Validate(nil))
}

func TestBeginExecution_Validate_AbsentWithAvailableInNoAbsent_ReturnsError(t *testing.T) {
	cmd := BeginExecution{
		Namespace:   "github.com/org/repo",
		Method:      "_execute",
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	}
	err := cmd.Validate(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestBeginExecution_Validate_NoExecution_NoAvailableIn_ReturnsNil(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_execute"}
	rt := &domainRuntime.ArrowRuntime{
		Ref:   "github.com/org/repo",
		State: domain.ArrowStateReady,
	}
	require.NoError(t, cmd.Validate(rt))
}

func TestBeginExecution_EmitEvent_UpdateMethod_SetsUpdating(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_update"}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.ArrowStateUpdating, result.State)
}

func TestBeginExecution_EmitEvent_UninstallMethod_SetsUninstalling(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_uninstall"}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.ArrowStateUninstalling, result.State)
}

func TestBeginExecution_EmitEvent_NamedMethod_SetsRunning(t *testing.T) {
	step := domainStep.NewRunStep("deploy", "make deploy", false, "10s", true)
	cmd := BeginExecution{
		Namespace: "github.com/org/repo",
		Method:    "deploy",
		Steps:     []domainStep.Step{step},
	}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.ArrowStateRunning, result.State)
	require.NotNil(t, result.Execution)
	assert.Equal(t, "deploy", result.Execution.Method)
	require.Len(t, result.Execution.Steps, 1)
	assert.Equal(t, domainRuntime.StepStatusPending, result.Execution.Steps[0].Status)
}

func TestBeginExecution_EmitEvent_PreservesLastReturn_WhenNil(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_execute"}
	rt := &domainRuntime.ArrowRuntime{
		Ref:        "github.com/org/repo",
		State:      domain.ArrowStateReady,
		LastReturn: nil,
	}
	result := cmd.EmitEvent(rt)
	assert.Nil(t, result.LastReturn)
}

func TestBeginExecution_EmitEvent_PreservesLastReturn_WhenNonNil(t *testing.T) {
	lastRet := &domainRuntime.Return{
		Method:  "_install",
		Outcome: domainRuntime.ExecutionOutcomeSuccess,
	}
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_execute"}
	rt := &domainRuntime.ArrowRuntime{
		Ref:        "github.com/org/repo",
		State:      domain.ArrowStateReady,
		LastReturn: lastRet,
	}
	result := cmd.EmitEvent(rt)
	assert.Equal(t, lastRet, result.LastReturn)
}
