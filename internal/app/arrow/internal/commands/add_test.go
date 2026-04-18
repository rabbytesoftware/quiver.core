package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddArrow_AggregateID(t *testing.T) {
	cmd := AddArrow{Namespace: "github.com/org/repo"}
	assert.Equal(t, "github.com/org/repo", cmd.AggregateID())
}

func TestAddArrow_EventName(t *testing.T) {
	assert.Equal(t, "arrow.added", AddArrow{}.EventName())
}

func TestAddArrow_Validate_NilState_ReturnsNil(t *testing.T) {
	cmd := AddArrow{Namespace: "github.com/org/repo"}
	require.NoError(t, cmd.Validate(nil))
}

func TestAddArrow_Validate_AlreadyExists_ReturnsValidationError(t *testing.T) {
	cmd := AddArrow{Namespace: "github.com/org/repo"}
	existing := &domain.Arrow{Namespace: "github.com/org/repo"}
	err := cmd.Validate(existing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestAddArrow_EmitEvent_ReturnsArrow(t *testing.T) {
	manifest := domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"}}
	cmd := AddArrow{Namespace: "github.com/org/repo", Manifest: manifest}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result.Namespace)
	got, ok := result.Versions["1.0.0"]
	require.True(t, ok)
	assert.Equal(t, "Test", got.Name)
	assert.Equal(t, "1.0.0", got.Version)
}

func TestAddArrowCmd_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := AddArrow{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}
