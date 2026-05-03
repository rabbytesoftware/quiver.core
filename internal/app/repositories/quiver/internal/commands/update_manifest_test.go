package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestUpdateQuiverManifest_AggregateID(t *testing.T) {
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestUpdateQuiverManifest_EventName(t *testing.T) {
	assert.Equal(t, "quiver.updated", UpdateQuiverManifest{}.EventName())
}

func TestUpdateQuiverManifest_Validate_NilState_ReturnsError(t *testing.T) {
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	err := cmd.Validate(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestUpdateQuiverManifest_Validate_Active_ReturnsNil(t *testing.T) {
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	existing := &domain.Quiver{Namespace: "github.com/org/repo"}
	require.NoError(t, cmd.Validate(existing))
}

func TestUpdateQuiverManifest_EmitEvent_PreservesState(t *testing.T) {
	followedAt := time.Now()
	existing := &domain.Quiver{
		Namespace:  "github.com/org/repo",
		FollowedAt: followedAt,
	}
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	result := cmd.EmitEvent(existing)
	assert.Equal(t, existing.Namespace, result.Namespace)
	assert.Equal(t, followedAt.Unix(), result.FollowedAt.Unix())
}

func TestUpdateQuiverManifest_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}
