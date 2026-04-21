package commands_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetUserInstalled_AggregateID(t *testing.T) {
	cmd := commands.SetUserInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	assert.Equal(t, "github.com/org/repo@v1.0.0", cmd.AggregateID())
}

func TestSetUserInstalled_EventName(t *testing.T) {
	cmd := commands.SetUserInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	assert.Equal(t, "arrow.user_installed", cmd.EventName())
}

func TestSetUserInstalled_Validate_NilCurrent_ReturnsError(t *testing.T) {
	cmd := commands.SetUserInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	require.Error(t, cmd.Validate(nil))
}

func TestSetUserInstalled_Validate_ExistsCurrent_ReturnsNil(t *testing.T) {
	cmd := commands.SetUserInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	current := &domain.Arrow{Namespace: "github.com/org/repo@v1.0.0"}
	require.NoError(t, cmd.Validate(current))
}

func TestSetUserInstalled_EmitEvent_FlipsUserInstalled(t *testing.T) {
	cmd := commands.SetUserInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	current := &domain.Arrow{
		Namespace:     "github.com/org/repo@v1.0.0",
		UserInstalled: false,
		InstalledRef:  "v1.0.0",
	}
	result := cmd.EmitEvent(current)
	assert.True(t, result.UserInstalled)
	assert.Equal(t, "v1.0.0", result.InstalledRef) // preserved
}

func TestSetUserInstalled_EmitEvent_AlreadyTrue_StaysTrue(t *testing.T) {
	cmd := commands.SetUserInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	current := &domain.Arrow{
		Namespace:     "github.com/org/repo@v1.0.0",
		UserInstalled: true,
	}
	result := cmd.EmitEvent(current)
	assert.True(t, result.UserInstalled)
}
