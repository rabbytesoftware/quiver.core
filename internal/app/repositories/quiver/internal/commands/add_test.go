package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddQuiver_AggregateID(t *testing.T) {
	cmd := AddQuiver{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestAddQuiver_EventName(t *testing.T) {
	assert.Equal(t, "quiver.added", AddQuiver{}.EventName())
}

func TestAddQuiver_Validate_NilState_ReturnsNil(t *testing.T) {
	cmd := AddQuiver{Namespace: "github.com/org/repo"}
	require.NoError(t, cmd.Validate(nil))
}

func TestAddQuiver_Validate_AlreadyExists_ReturnsValidationError(t *testing.T) {
	cmd := AddQuiver{Namespace: "github.com/org/repo"}
	existing := &domain.Quiver{Namespace: "github.com/org/repo"}
	err := cmd.Validate(existing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestAddQuiver_EmitEvent_ReturnsQuiver(t *testing.T) {
	manifest := domain.QuiverManifest{Name: "Test"}
	cmd := AddQuiver{Namespace: "github.com/org/repo", Manifest: manifest}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result.Namespace)
	assert.Equal(t, manifest, result.Manifest)
}

func TestAddQuiver_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := AddQuiver{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}
