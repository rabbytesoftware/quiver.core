package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
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

func TestGetSearch_Defaults(t *testing.T) {
	resetForTesting()
	s := GetSearch()
	assert.Equal(t, 25, s.PerProviderLimit)
	assert.Equal(t, 8, s.FetchConcurrency)
	assert.Equal(t, "10s", s.ProviderTimeout)
}

func TestGetSearch_ProviderTimeout_ParseableAsDuration(t *testing.T) {
	resetForTesting()
	_, err := time.ParseDuration(GetSearch().ProviderTimeout)
	assert.NoError(t, err)
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
			os.WriteFile(path, original, 0o644)
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	// Partial override — only api.host; all other fields keep defaults.
	require.NoError(t, os.WriteFile(path, []byte(`config:
  api:
    host: "tcp://test-host:9999"
`), 0o644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.Equal(t, "tcp://test-host:9999", cfg.Config.API.Host)
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
			os.WriteFile(path, original, 0o644)
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("not: [valid: yaml\x00"), 0o644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
}

func TestGet_InvalidField_FallsBackAloneAndKeepsSiblings(t *testing.T) {
	path := metadata.GetConfigPath()
	original, originalErr := os.ReadFile(path)
	t.Cleanup(func() {
		resetForTesting()
		if originalErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0o600)
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(`config:
  api:
    host: "tcp://test-host:9999"
  vault:
    ttl: "banana"
`), 0o600))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.Equal(t, "tcp://test-host:9999", cfg.Config.API.Host)
	assert.Equal(t, Defaults().Vault.TTL, cfg.Config.Vault.TTL)
}
