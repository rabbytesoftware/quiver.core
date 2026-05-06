package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestFollowQuiver_AggregateID(t *testing.T) {
	cmd := FollowQuiver{Quiver: domain.Quiver{Namespace: "github.com/org/repo"}}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestFollowQuiver_EventName(t *testing.T) {
	assert.Equal(t, "quiver.followed", FollowQuiver{}.EventName())
}

func TestFollowQuiver_Validate_NilState_ReturnsNil(t *testing.T) {
	cmd := FollowQuiver{Quiver: domain.Quiver{Namespace: "github.com/org/repo"}}
	require.NoError(t, cmd.Validate(nil))
}

func TestFollowQuiver_Validate_AlreadyExists_ReturnsValidationError(t *testing.T) {
	cmd := FollowQuiver{Quiver: domain.Quiver{Namespace: "github.com/org/repo"}}
	existing := &domain.Quiver{Namespace: "github.com/org/repo"}
	err := cmd.Validate(existing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestFollowQuiver_EmitEvent_ReturnsQuiver(t *testing.T) {
	cmd := FollowQuiver{Quiver: domain.Quiver{Namespace: "github.com/org/repo"}}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result.Namespace)
	assert.False(t, result.FollowedAt.IsZero())
}

func TestFollowQuiver_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := FollowQuiver{Quiver: domain.Quiver{Namespace: "github.com/org/repo"}}
	assert.True(t, cmd.ShouldSnapshot())
}
