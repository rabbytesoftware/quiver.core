package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	assert.NotEmpty(t, cfg.Config.Logger.Level)
	assert.NotEmpty(t, cfg.Config.Manifold.FetchTimeout)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortStart)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortEnd)
}

func TestGetNetbridge_FieldsAccessible(t *testing.T) {
	nb := GetNetbridge()
	_ = nb.Enabled
	_ = nb.EphemeralPortStart
	_ = nb.EphemeralPortEnd
}

func TestGetAPI_ValidValues(t *testing.T) {
	api := GetAPI()
	assert.NotEmpty(t, api.Host)
	assert.Positive(t, api.Port)
}

func TestGetLogger_ValidValues(t *testing.T) {
	lg := GetLogger()
	assert.NotEmpty(t, lg.Level)
}

func TestGetManifold_FetchTimeout_ParseableAsDuration(t *testing.T) {
	m := GetManifold()
	assert.NotEmpty(t, m.FetchTimeout)
	_, err := time.ParseDuration(m.FetchTimeout)
	assert.NoError(t, err, "FetchTimeout %q must be parseable by time.ParseDuration", m.FetchTimeout)
}

func TestGetVault_DefaultSweepInterval(t *testing.T) {
	resetForTesting()
	assert.Equal(t, "5m", GetVault().SweepInterval)
}

func TestGetVault_DefaultTTL(t *testing.T) {
	resetForTesting()
	assert.Equal(t, "24h", GetVault().TTL)
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
	// Logger defaults must survive the partial override.
	assert.NotEmpty(t, cfg.Config.Logger.Level)
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
