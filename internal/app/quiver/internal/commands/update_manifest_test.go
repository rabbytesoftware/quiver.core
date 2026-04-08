package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestUpdateQuiverManifest_Validate_Removed_ReturnsError(t *testing.T) {
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	existing := &domain.Quiver{Namespace: "github.com/org/repo", Removed: true}
	err := cmd.Validate(existing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestUpdateQuiverManifest_Validate_Active_ReturnsNil(t *testing.T) {
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	existing := &domain.Quiver{Namespace: "github.com/org/repo", Removed: false}
	require.NoError(t, cmd.Validate(existing))
}

func TestUpdateQuiverManifest_EmitEvent_UpdatesManifest(t *testing.T) {
	newManifest := domain.QuiverManifest{Name: "Updated"}
	existing := &domain.Quiver{Namespace: "github.com/org/repo", Removed: false}
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo", Manifest: newManifest}
	result := cmd.EmitEvent(existing)
	assert.Equal(t, newManifest, result.Manifest)
	assert.False(t, result.Removed)
}

func TestUpdateQuiverManifestCmd_ShouldSnapshot_ReturnsFalse(t *testing.T) {
	cmd := UpdateQuiverManifest{Namespace: "github.com/org/repo"}
	assert.False(t, cmd.ShouldSnapshot())
}
