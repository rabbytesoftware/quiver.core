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

func TestAdvanceStep_Validate_NoActiveRun_ReturnsError(t *testing.T) {
	cmd := AdvanceStep{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{Namespace: "github.com/org/repo"}
	err := cmd.Validate(rt)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestAdvanceStep_EmitEvent_UpdatesStepStatus(t *testing.T) {
	status := domainRuntime.StepStatusCompleted
	cmd := AdvanceStep{
		Namespace: "github.com/org/repo",
		StepIndex: 0,
		ToStatus:  status,
	}
	rt := &domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		State:     domain.ArrowStateInstalling,
		ActiveRun: &domainRuntime.RunRecord{
			Method: "_install",
			Steps: []domainRuntime.StepProgress{
				{Index: 0, Status: domainRuntime.StepStatusPending},
			},
		},
	}
	result := cmd.EmitEvent(rt)
	require.NotNil(t, result.ActiveRun)
	assert.Equal(t, status, result.ActiveRun.Steps[0].Status)
}
