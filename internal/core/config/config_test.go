package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// A dev build points QUIVER_HOME at a checkout-local directory so a run never
// touches the real ~/.quiver — these tests use the same override to read and
// write their config.yaml fixtures under t.TempDir() instead of the
// developer's actual config file.

// GetAt (below) is the WithHomeDir-threaded sibling core.NewAt uses so a test
// container built with internal.WithHomeDir(t.TempDir()) never touches the
// real ~/.quiver's config.yaml. Unlike Get, it is never cached: caching a
// single result would freeze the first homeDir's config for every later
// caller with a different homeDir in the same process.

func TestGetAt_MissingFile_UsesDefaults(t *testing.T) {
	cfg, corrections := GetAt(t.TempDir())
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
	assert.Empty(t, corrections)
}

func TestGetAt_WithValidConfigFile_MergesOverrides(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`config:
  api:
    host: "tcp://at-host:2222"
`), 0o644))

	cfg, _ := GetAt(dir)
	require.NotNil(t, cfg)
	assert.Equal(t, "tcp://at-host:2222", cfg.Config.API.Host)
}

func TestGetAt_WithInvalidYAML_FallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("not: [valid: yaml\x00"), 0o644))

	cfg, corrections := GetAt(dir)
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
	assert.Empty(t, corrections)
}

func TestGetAt_InvalidField_ReportsCorrection(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("config:\n  vault:\n    ttl: banana\n"), 0o644))

	cfg, corrections := GetAt(dir)
	require.NotNil(t, cfg)
	assert.Equal(t, Defaults().Vault.TTL, cfg.Config.Vault.TTL)
	require.Len(t, corrections, 1)
	assert.Equal(t, "vault.ttl", corrections[0].Key)
}

func TestGetAt_DoesNotShareStateAcrossDifferentHomeDirs(t *testing.T) {
	dirA := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dirA, "config.yaml"), []byte(`config:
  api:
    host: "tcp://host-a:1111"
`), 0o644))
	dirB := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dirB, "config.yaml"), []byte(`config:
  api:
    host: "tcp://host-b:2222"
`), 0o644))

	cfgA, _ := GetAt(dirA)
	cfgB, _ := GetAt(dirB)

	assert.Equal(t, "tcp://host-a:1111", cfgA.Config.API.Host)
	assert.Equal(t, "tcp://host-b:2222", cfgB.Config.API.Host)
}

func TestGet_QuiverHomeEnvSet_ReadsScopedConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUIVER_HOME", dir)
	t.Cleanup(resetForTesting)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`config:
  api:
    host: "tcp://scoped-host:1111"
`), 0o644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.Equal(t, "tcp://scoped-host:1111", cfg.Config.API.Host)
}

func TestGet_WithValidConfigFile_MergesOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUIVER_HOME", dir)
	t.Cleanup(resetForTesting)

	// Partial override — only api.host; all other fields keep defaults.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`config:
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
	dir := t.TempDir()
	t.Setenv("QUIVER_HOME", dir)
	t.Cleanup(resetForTesting)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("not: [valid: yaml\x00"), 0o644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
}

func TestGet_InvalidField_FallsBackAloneAndKeepsSiblings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUIVER_HOME", dir)
	t.Cleanup(resetForTesting)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`config:
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

func TestGetArrows_ReturnsAutoRetrySection(t *testing.T) {
	assert.GreaterOrEqual(t, GetArrows().AutoRetry.Retries, 0)
}

func TestGetAuth_Defaults(t *testing.T) {
	resetForTesting()
	a := GetAuth()
	assert.Equal(t, "5m", a.PairingCodeTTL)
	assert.Equal(t, 5, a.RedeemRateLimit)
	assert.Equal(t, "1m", a.RedeemRateWindow)
}

func TestGetAuth_DurationsAreParseable(t *testing.T) {
	resetForTesting()
	a := GetAuth()
	_, err := time.ParseDuration(a.PairingCodeTTL)
	assert.NoError(t, err)
	_, err = time.ParseDuration(a.RedeemRateWindow)
	assert.NoError(t, err)
}

func TestCorrections_ReportsWhatTheLoadReplaced(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUIVER_HOME", dir)
	t.Cleanup(resetForTesting)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("config:\n  vault:\n    ttl: banana\n"), 0o600))

	resetForTesting()
	corrections := Corrections()

	require.Len(t, corrections, 1)
	assert.Equal(t, "vault.ttl", corrections[0].Key)
}

func TestCorrections_CleanFileReportsNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUIVER_HOME", dir)
	t.Cleanup(resetForTesting)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("config:\n  vault:\n    ttl: 48h\n"), 0o600))

	resetForTesting()
	assert.Empty(t, Corrections())
}
