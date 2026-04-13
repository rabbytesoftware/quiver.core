package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_ReturnsSingleton(t *testing.T) {
	resetForTesting()
	cfg := Get()
	require.NotNil(t, cfg)
	assert.Same(t, cfg, Get())
}

func TestGet_DefaultsPopulated(t *testing.T) {
	resetForTesting()
	cfg := Get()
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
	assert.Positive(t, cfg.Config.API.Port)
	assert.NotEmpty(t, cfg.Config.Database.Path)
	assert.NotEmpty(t, cfg.Config.Watcher.Level)
	assert.NotEmpty(t, cfg.Config.Watcher.Folder)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortStart)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortEnd)
}

func TestGetNetbridge_FieldsAccessible(t *testing.T) {
	nb := GetNetbridge()
	_ = nb.Enabled
	_ = nb.EphemeralPortStart
	_ = nb.EphemeralPortEnd
}

func TestGetArrows_FieldsAccessible(t *testing.T) {
	arrows := GetArrows()
	_ = arrows.Repositories
	_ = arrows.InstallDir
}

func TestGetAPI_ValidValues(t *testing.T) {
	api := GetAPI()
	assert.NotEmpty(t, api.Host)
	assert.Positive(t, api.Port)
}

func TestGetDatabase_ValidPath(t *testing.T) {
	assert.NotEmpty(t, GetDatabase().Path)
}

func TestGetWatcher_ValidValues(t *testing.T) {
	w := GetWatcher()
	assert.NotEmpty(t, w.Level)
	assert.NotEmpty(t, w.Folder)
}

func TestGetDefaultConfig_NeverNil(t *testing.T) {
	require.NotNil(t, getDefaultConfig())
}

func TestGet_MissingFile_UsesDefaults(t *testing.T) {
	resetForTesting()
	cfg := Get()
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
}

func TestGet_WithValidConfigFile_MergesOverrides(t *testing.T) {
	path := metadata.GetConfigPath()
	original, originalErr := os.ReadFile(path)
	t.Cleanup(func() {
		resetForTesting()
		if originalErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0644)
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))

	// Partial override — only api.host and api.port; all other fields keep defaults.
	require.NoError(t, os.WriteFile(path, []byte(`config:
  api:
    host: "test-host"
    port: 9999
`), 0644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.Equal(t, "test-host", cfg.Config.API.Host)
	assert.Equal(t, 9999, cfg.Config.API.Port)
	// Watcher should still have defaults since it wasn't in the partial override.
	assert.NotEmpty(t, cfg.Config.Watcher.Level)
}

func TestGet_WithInvalidYAML_FallsBackToDefaults(t *testing.T) {
	path := metadata.GetConfigPath()
	original, originalErr := os.ReadFile(path)
	t.Cleanup(func() {
		resetForTesting()
		if originalErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0644)
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte("not: [valid: yaml\x00"), 0644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
}
