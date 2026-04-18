package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateArrowManifest_AggregateID_ReturnsBareNamespace(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo@v2.0.0"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
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

func TestUpdateArrowManifest_EmitEvent_NoRef_KeyedAsLatest(t *testing.T) {
	newVersion := domain.ArrowVersion{ArrowMeta: domain.ArrowMeta{Name: "Updated", Version: "2.0.0"}}
	existing := &domain.Arrow{Namespace: "github.com/org/repo"}
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo", Version: newVersion}
	result := cmd.EmitEvent(existing)
	got, ok := result.Versions["latest"]
	require.True(t, ok)
	assert.Equal(t, newVersion.ArrowMeta, got.ArrowMeta)
	assert.Equal(t, "", got.InstalledRef)
}

func TestUpdateArrowManifest_EmitEvent_WithRef_KeyedByRef(t *testing.T) {
	newVersion := domain.ArrowVersion{ArrowMeta: domain.ArrowMeta{Name: "Updated", Version: "2.0.0"}}
	existing := &domain.Arrow{Namespace: "github.com/org/repo"}
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo@v2.0.0", Version: newVersion}
	result := cmd.EmitEvent(existing)
	got, ok := result.Versions["v2.0.0"]
	require.True(t, ok)
	assert.Equal(t, newVersion.ArrowMeta, got.ArrowMeta)
	assert.Equal(t, "v2.0.0", got.InstalledRef)
}

func TestUpdateArrowManifest_EmitEvent_PreservesExistingVersions(t *testing.T) {
	newVersion := domain.ArrowVersion{ArrowMeta: domain.ArrowMeta{Name: "Updated", Version: "2.0.0"}}
	existing := &domain.Arrow{
		Namespace: "github.com/org/repo",
		Versions: map[string]domain.ArrowVersion{
			"v1.0.0": {ArrowMeta: domain.ArrowMeta{Name: "Old", Version: "1.0.0"}},
		},
	}
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo@v2.0.0", Version: newVersion}
	result := cmd.EmitEvent(existing)
	assert.Len(t, result.Versions, 2)
	_, hasOld := result.Versions["v1.0.0"]
	assert.True(t, hasOld)
}

func TestUpdateArrowManifestCmd_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := UpdateArrowManifest{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}
