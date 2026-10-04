//go:build integration

package selfupdate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

func TestSelfUpdate_Harness_NeverReadsTheDevelopersQuiverHome(t *testing.T) {
	userHome, err := os.UserHomeDir()
	require.NoError(t, err)

	home := metadata.GetHomePath()

	assert.NotEqual(t, filepath.Join(userHome, ".quiver"), home, "the suite must run against its own home, not the real one")
	assert.Empty(t, config.GetArrows().SelfUpdateChannel,
		"a developer's own self_update_channel overrides the build channel the suite stamps and changes the self-arrow's identity")
}
