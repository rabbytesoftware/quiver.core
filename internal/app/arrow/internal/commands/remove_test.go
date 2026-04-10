package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveArrow_AggregateID(t *testing.T) {
	cmd := RemoveArrow{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestRemoveArrow_EventName(t *testing.T) {
	assert.Equal(t, "arrow.removed", RemoveArrow{}.EventName())
}

func TestRemoveArrow_Validate_NilState_ReturnsError(t *testing.T) {
	cmd := RemoveArrow{Namespace: "github.com/org/repo"}
	err := cmd.Validate(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestRemoveArrow_Validate_AlreadyRemoved_ReturnsError(t *testing.T) {
	cmd := RemoveArrow{Namespace: "github.com/org/repo"}
	existing := &domain.Arrow{Namespace: "github.com/org/repo", Removed: true}
	err := cmd.Validate(existing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestRemoveArrow_Validate_Active_ReturnsNil(t *testing.T) {
	cmd := RemoveArrow{Namespace: "github.com/org/repo"}
	existing := &domain.Arrow{Namespace: "github.com/org/repo", Removed: false}
	require.NoError(t, cmd.Validate(existing))
}

func TestRemoveArrow_EmitEvent_MarksRemoved(t *testing.T) {
	existing := &domain.Arrow{Namespace: "github.com/org/repo", Removed: false}
	cmd := RemoveArrow{Namespace: "github.com/org/repo"}
	result := cmd.EmitEvent(existing)
	assert.True(t, result.Removed)
}

func TestRemoveArrowCmd_ShouldSnapshot_ReturnsFalse(t *testing.T) {
	cmd := RemoveArrow{Namespace: "github.com/org/repo"}
	assert.False(t, cmd.ShouldSnapshot())
}
