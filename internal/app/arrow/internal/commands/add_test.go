package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddArrow_AggregateID_ReturnsFullNamespace(t *testing.T) {
	cmd := AddArrow{Namespace: "github.com/org/repo@v1.2.3"}
	assert.Equal(t, "github.com/org/repo@v1.2.3", cmd.AggregateID())
}

func TestAddArrow_AggregateID_NoRef_ReturnsBareNamespace(t *testing.T) {
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

func TestAddArrow_EmitEvent_PopulatesFields(t *testing.T) {
	cmd := AddArrow{
		Namespace: "github.com/org/repo",
		ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
		DirectInstall:       true,
		InstalledConstraint: ">=1.0",
	}
	result := cmd.EmitEvent(nil)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result.Namespace)
	assert.Equal(t, "Test", result.Name)
	assert.Equal(t, "1.0.0", result.Version)
	assert.True(t, result.UserInstalled)
	assert.Equal(t, ">=1.0", result.InstalledConstraint)
	_, ok := result.Targets[domain.OSLinuxAMD64]
	assert.True(t, ok)
}

func TestAddArrow_EmitEvent_IndirectInstall_NotDirectInstall(t *testing.T) {
	cmd := AddArrow{
		Namespace: "github.com/org/repo",
		ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"},
	}
	result := cmd.EmitEvent(nil)
	assert.False(t, result.UserInstalled)
}

func TestAddArrowCmd_ShouldSnapshot_ReturnsTrue(t *testing.T) {
	cmd := AddArrow{Namespace: "github.com/org/repo"}
	assert.True(t, cmd.ShouldSnapshot())
}
