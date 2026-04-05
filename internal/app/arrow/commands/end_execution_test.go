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

func TestEndExecution_AggregateID(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestEndExecution_EventName(t *testing.T) {
	assert.Equal(t, "runtime.ended", EndExecution{}.EventName())
}

func TestEndExecution_Validate_NilRuntime_ReturnsError(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo"}
	err := cmd.Validate(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestEndExecution_Validate_NoActiveRun_ReturnsError(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{Namespace: "github.com/org/repo", State: domain.ArrowStateReady}
	err := cmd.Validate(rt)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestEndExecution_Validate_WithActiveRun_ReturnsNil(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{Method: "_execute"},
	}
	require.NoError(t, cmd.Validate(rt))
}

func TestEndExecution_EmitEvent_InstallSuccess_SetsReady(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo", Outcome: domainRuntime.ExecutionOutcomeSuccess}
	rt := &domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		ActiveRun: &domainRuntime.RunRecord{Method: "_install"},
	}
	result := cmd.EmitEvent(rt)
	assert.Equal(t, domain.ArrowStateReady, result.State)
	assert.Nil(t, result.ActiveRun)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, result.LastReturn.Outcome)
}

func TestEndExecution_EmitEvent_InstallFailed_SetsAbsent(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo", Outcome: domainRuntime.ExecutionOutcomeFailed}
	rt := &domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		ActiveRun: &domainRuntime.RunRecord{Method: "_install"},
	}
	result := cmd.EmitEvent(rt)
	assert.Equal(t, domain.ArrowStateAbsent, result.State)
}

func TestEndExecutionCmd_ShouldSnapshot_ReturnsFalse(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo"}
	assert.False(t, cmd.ShouldSnapshot())
}

func TestEndExecution_EmitEvent_UninstallSuccess_SetsAbsent(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo", Outcome: domainRuntime.ExecutionOutcomeSuccess}
	rt := &domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		ActiveRun: &domainRuntime.RunRecord{Method: "_uninstall"},
	}
	result := cmd.EmitEvent(rt)
	assert.Equal(t, domain.ArrowStateAbsent, result.State)
	assert.Nil(t, result.ActiveRun)
}

func TestEndExecution_EmitEvent_UninstallFailed_SetsReady(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo", Outcome: domainRuntime.ExecutionOutcomeFailed}
	rt := &domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		ActiveRun: &domainRuntime.RunRecord{Method: "_uninstall"},
	}
	result := cmd.EmitEvent(rt)
	assert.Equal(t, domain.ArrowStateReady, result.State)
}

func TestEndExecution_EmitEvent_DefaultMethod_Cancelled_SetsReady(t *testing.T) {
	cmd := EndExecution{Namespace: "github.com/org/repo", Outcome: domainRuntime.ExecutionOutcomeCancelled}
	rt := &domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		ActiveRun: &domainRuntime.RunRecord{Method: "_execute"},
	}
	result := cmd.EmitEvent(rt)
	assert.Equal(t, domain.ArrowStateReady, result.State)
	assert.Nil(t, result.ActiveRun)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, result.LastReturn.Outcome)
}
