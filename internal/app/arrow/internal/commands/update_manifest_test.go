package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateArrowManifest_AggregateID(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestUpdateArrowManifest_EventName(t *testing.T) {
	assert.Equal(t, "arrow.updated", UpdateArrowManifest{}.EventName())
}

func TestUpdateArrowManifest_Validate_NilState_ReturnsError(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	err := cmd.Validate(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestUpdateArrowManifest_Validate_Removed_ReturnsError(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	existing := &domain.Arrow{Namespace: "github.com/org/repo", Removed: true}
	err := cmd.Validate(existing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestUpdateArrowManifest_Validate_Active_ReturnsNil(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	existing := &domain.Arrow{Namespace: "github.com/org/repo", Removed: false}
	require.NoError(t, cmd.Validate(existing))
}

func TestUpdateArrowManifest_EmitEvent_UpdatesManifest(t *testing.T) {
	newManifest := domain.ArrowManifest{Name: "Updated", Version: "2.0.0"}
	existing := &domain.Arrow{Namespace: "github.com/org/repo", Removed: false}
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo", Manifest: newManifest}
	result := cmd.EmitEvent(existing)
	assert.Equal(t, newManifest, result.Manifest)
	assert.False(t, result.Removed)
}

func TestUpdateArrowManifestCmd_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}
