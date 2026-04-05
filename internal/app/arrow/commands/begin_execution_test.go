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
		Namespace: "github.com/org/repo",
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{Method: "_execute"},
	}
	err := cmd.Validate(rt)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestBeginExecution_EmitEvent_SetsRunningState(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_execute"}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.ArrowStateRunning, result.State)
	require.NotNil(t, result.ActiveRun)
	assert.Equal(t, "_execute", result.ActiveRun.Method)
}

func TestBeginExecution_EmitEvent_InstallMethod_SetsInstalling(t *testing.T) {
	cmd := BeginExecution{Namespace: "github.com/org/repo", Method: "_install"}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.ArrowStateInstalling, result.State)
}
