package commands_test

import (
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkInstalled_AggregateID(t *testing.T) {
	cmd := commands.MarkInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	assert.Equal(t, "github.com/org/repo@v1.0.0", cmd.AggregateID())
}

func TestMarkInstalled_EventName(t *testing.T) {
	cmd := commands.MarkInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	assert.Equal(t, "arrow.installed", cmd.EventName())
}

func TestMarkInstalled_ShouldSnapshot(t *testing.T) {
	cmd := commands.MarkInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	assert.True(t, cmd.ShouldSnapshot())
}

func TestMarkInstalled_Validate_NilCurrent_ReturnsError(t *testing.T) {
	cmd := commands.MarkInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	require.Error(t, cmd.Validate(nil))
}

func TestMarkInstalled_Validate_ExistsCurrent_ReturnsNil(t *testing.T) {
	cmd := commands.MarkInstalled{Namespace: "github.com/org/repo@v1.0.0"}
	current := &domain.Arrow{Namespace: "github.com/org/repo@v1.0.0"}
	require.NoError(t, cmd.Validate(current))
}

func TestMarkInstalled_EmitEvent_StampsInstalledAtAndRef(t *testing.T) {
	now := time.Now()
	cmd := commands.MarkInstalled{
		Namespace:    "github.com/org/repo@v1.0.0",
		InstalledAt:  now,
		InstalledRef: "v1.0.0",
	}
	current := &domain.Arrow{
		Namespace:           "github.com/org/repo@v1.0.0",
		InstalledConstraint: "v1.*",
		UserInstalled:       true,
	}
	result := cmd.EmitEvent(current)
	assert.Equal(t, now, result.InstalledAt)
	assert.Equal(t, "v1.0.0", result.InstalledRef)
	assert.Equal(t, "v1.*", result.InstalledConstraint) // preserved
	assert.True(t, result.UserInstalled)                // preserved
}
