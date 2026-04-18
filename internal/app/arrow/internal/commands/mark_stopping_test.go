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

func TestMarkStopping_AggregateID(t *testing.T) {
	cmd := MarkStopping{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestMarkStopping_EventName(t *testing.T) {
	assert.Equal(t, "runtime.mark_stopping", MarkStopping{}.EventName())
}

func TestMarkStopping_Validate_NilRuntime_ReturnsError(t *testing.T) {
	cmd := MarkStopping{Namespace: "github.com/org/repo"}
	err := cmd.Validate(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestMarkStopping_Validate_NotRunning_ReturnsError(t *testing.T) {
	cmd := MarkStopping{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{Ref: "github.com/org/repo", State: domain.ArrowStateReady}
	err := cmd.Validate(rt)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestMarkStopping_Validate_Running_ReturnsNil(t *testing.T) {
	cmd := MarkStopping{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{Ref: "github.com/org/repo", State: domain.ArrowStateRunning}
	require.NoError(t, cmd.Validate(rt))
}

func TestMarkStopping_EmitEvent_SetsStopping(t *testing.T) {
	cmd := MarkStopping{Namespace: "github.com/org/repo"}
	rt := &domainRuntime.ArrowRuntime{Ref: "github.com/org/repo", State: domain.ArrowStateRunning}
	result := cmd.EmitEvent(rt)
	assert.Equal(t, domain.ArrowStateStopping, result.State)
}

func TestMarkStoppingCmd_ShouldSnapshot_ReturnsFalse(t *testing.T) {
	cmd := MarkStopping{Namespace: "github.com/org/repo"}
	assert.False(t, cmd.ShouldSnapshot())
}
