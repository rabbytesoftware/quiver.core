package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestFollowCollection_AggregateID(t *testing.T) {
	cmd := FollowCollection{Collection: domain.Collection{Namespace: "github.com/org/repo"}}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestFollowCollection_EventName(t *testing.T) {
	assert.Equal(t, "collection.followed", FollowCollection{}.EventName())
}

func TestFollowCollection_Validate_NilState_ReturnsNil(t *testing.T) {
	cmd := FollowCollection{Collection: domain.Collection{Namespace: "github.com/org/repo"}}
	require.NoError(t, cmd.Validate(nil))
}

func TestFollowCollection_Validate_AlreadyExists_ReturnsValidationError(t *testing.T) {
	cmd := FollowCollection{Collection: domain.Collection{Namespace: "github.com/org/repo"}}
	existing := &domain.Collection{Namespace: "github.com/org/repo"}
	err := cmd.Validate(existing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestFollowCollection_EmitEvent_ReturnsQuiver(t *testing.T) {
	cmd := FollowCollection{Collection: domain.Collection{Namespace: "github.com/org/repo"}}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result.Namespace)
	assert.False(t, result.FollowedAt.IsZero())
}

func TestFollowCollection_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := FollowCollection{Collection: domain.Collection{Namespace: "github.com/org/repo"}}
	assert.True(t, cmd.ShouldSnapshot())
}
