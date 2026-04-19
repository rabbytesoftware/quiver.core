package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateArrowManifest_AggregateID_ReturnsFullNamespace(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo@v2.0.0"}
	assert.Equal(t, "github.com/org/repo@v2.0.0", cmd.AggregateID())
}

func TestUpdateArrowManifest_AggregateID_NoRef_ReturnsBareNamespace(t *testing.T) {
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

func TestUpdateArrowManifest_Validate_Active_ReturnsNil(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	existing := &domain.Arrow{Namespace: "github.com/org/repo"}
	require.NoError(t, cmd.Validate(existing))
}

func TestUpdateArrowManifest_EmitEvent_UpdatesFields(t *testing.T) {
	existing := &domain.Arrow{
		Namespace: "github.com/org/repo",
		ArrowMeta: domain.ArrowMeta{Name: "Old", Version: "1.0.0"},
	}
	cmd := UpdateArrowManifest{
		Namespace: "github.com/org/repo",
		ArrowMeta: domain.ArrowMeta{Name: "Updated", Version: "2.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
	}
	result := cmd.EmitEvent(existing)
	assert.Equal(t, "Updated", result.Name)
	assert.Equal(t, "2.0.0", result.Version)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result.Namespace)
	_, ok := result.Targets[domain.OSLinuxAMD64]
	assert.True(t, ok)
}

func TestUpdateArrowManifestCmd_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}
