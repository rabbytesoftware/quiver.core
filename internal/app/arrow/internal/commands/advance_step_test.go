package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvanceStep_AggregateID(t *testing.T) {
	cmd := AdvanceStep{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestAdvanceStep_EventName(t *testing.T) {
	assert.Equal(t, "runtime.step_advanced", AdvanceStep{}.EventName())
}

func TestAdvanceStep_Validate_NilRuntime_ReturnsError(t *testing.T) {
	cmd := AdvanceStep{Namespace: "github.com/org/repo"}
	err := cmd.Validate(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestAdvanceStep_Validate_NoExecution_ReturnsError(t *testing.T) {
	cmd := AdvanceStep{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{Ref: "github.com/org/repo"}
	err := cmd.Validate(rt)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestAdvanceStep_Validate_WithExecution_ReturnsNil(t *testing.T) {
	cmd := AdvanceStep{Namespace: "github.com/org/repo", StepIndex: 0}
	rt := &domainRuntime.ArrowRuntime{
		Ref:       "github.com/org/repo",
		State:     domain.ArrowStateInstalling,
		Execution: &domainRuntime.Execution{Method: "_install"},
	}
	require.NoError(t, cmd.Validate(rt))
}

func TestAdvanceStep_EmitEvent_UpdatesStepStatus(t *testing.T) {
	status := domainRuntime.StepStatusCompleted
	cmd := AdvanceStep{
		Namespace: "github.com/org/repo",
		StepIndex: 0,
		ToStatus:  status,
	}
	rt := &domainRuntime.ArrowRuntime{
		Ref:   "github.com/org/repo",
		State: domain.ArrowStateInstalling,
		Execution: &domainRuntime.Execution{
			Method: "_install",
			Steps: []domainRuntime.StepProgress{
				{Index: 0, Status: domainRuntime.StepStatusPending},
			},
		},
	}
	result := cmd.EmitEvent(rt)
	require.NotNil(t, result.Execution)
	assert.Equal(t, status, result.Execution.Steps[0].Status)
}

func TestAdvanceStepCmd_ShouldSnapshot_ReturnsFalse(t *testing.T) {
	cmd := AdvanceStep{Namespace: "github.com/org/repo"}
	assert.False(t, cmd.ShouldSnapshot())
}

func TestAdvanceStep_EmitEvent_SetsError(t *testing.T) {
	errMsg := "step failed: timeout"
	status := domainRuntime.StepStatusFailed
	cmd := AdvanceStep{
		Namespace: "github.com/org/repo",
		StepIndex: 0,
		ToStatus:  status,
		Error:     &errMsg,
	}
	rt := &domainRuntime.ArrowRuntime{
		Ref:   "github.com/org/repo",
		State: domain.ArrowStateInstalling,
		Execution: &domainRuntime.Execution{
			Method: "_install",
			Steps: []domainRuntime.StepProgress{
				{Index: 0, Status: domainRuntime.StepStatusRunning},
			},
		},
	}
	result := cmd.EmitEvent(rt)
	require.NotNil(t, result.Execution)
	assert.Equal(t, status, result.Execution.Steps[0].Status)
	require.NotNil(t, result.Execution.Steps[0].Error)
	assert.Equal(t, errMsg, *result.Execution.Steps[0].Error)
}
