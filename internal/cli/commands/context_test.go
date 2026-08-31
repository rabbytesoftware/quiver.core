package commands_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

func TestContextAdd_ServerFlagBindsToTheContext(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "cli.yaml")

	_, err := runCLIConfig(t, cfgPath, "context", "add", "staging", "--server", "tcp://localhost:40257")
	require.NoError(t, err)

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	got, err := cfg.Get("staging")
	require.NoError(t, err)
	assert.Equal(t, "tcp://localhost:40257", got.Server)
}

func TestContextAdd_CtxServerAliasStillWorks(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "cli.yaml")

	_, err := runCLIConfig(t, cfgPath, "context", "add", "legacy", "--ctx-server", "tcp://localhost:1")
	require.NoError(t, err)

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	got, err := cfg.Get("legacy")
	require.NoError(t, err)
	assert.Equal(t, "tcp://localhost:1", got.Server)
}

func TestContextAdd_MissingServerIsAUsageError(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")

	_, err := runCLIConfig(t, cfgPath, "context", "add", "nope")

	require.Error(t, err)
	assert.Equal(t, client.ExitUsage, commands.ExitCode(err))
}
