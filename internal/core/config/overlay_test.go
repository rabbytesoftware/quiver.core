package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

func TestConfiguredAt_MissingFile_ReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	data, corrected, err := ConfiguredAt(path)

	require.NoError(t, err)
	assert.Empty(t, corrected)
	assert.Equal(t, Defaults(), data)
}

func TestConfiguredAt_MalformedYAML_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("config:\n  netbridge: [oops\n"), 0o600))

	_, _, err := ConfiguredAt(path)

	require.Error(t, err)
}

func TestConfiguredAt_InvalidField_SanitizesAndReports(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("config:\n  vault:\n    ttl: banana\n"), 0o600))

	data, corrected, err := ConfiguredAt(path)

	require.NoError(t, err)
	require.Len(t, corrected, 1)
	assert.Equal(t, "vault.ttl", corrected[0].Key)
	assert.Equal(t, Defaults().Vault.TTL, data.Vault.TTL)
}

func TestSaveAt_RoundTripsNonDefaultFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	want := Defaults()
	want.Netbridge.EphemeralPortStart = 50000
	want.Logger.Level = "debug"

	require.NoError(t, SaveAt(path, want))

	got, corrected, err := ConfiguredAt(path)
	require.NoError(t, err)
	assert.Empty(t, corrected)
	assert.Equal(t, want, got)
}

func TestSaveAt_WritesOnlyNonDefaultLeaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	data := Defaults()
	data.Logger.Level = "debug"

	require.NoError(t, SaveAt(path, data))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(raw)
	assert.Contains(t, content, "logger:")
	assert.Contains(t, content, "level: debug")
	assert.NotContains(t, content, "netbridge:")
	assert.NotContains(t, content, "vault:")
	assert.NotContains(t, content, "search:")
}

func TestSaveAt_AllDefaults_WritesNoLeaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, SaveAt(path, Defaults()))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "level:")
	assert.NotContains(t, string(raw), "netbridge:")

	got, _, err := ConfiguredAt(path)
	require.NoError(t, err)
	assert.Equal(t, Defaults(), got)
}

func TestSaveAt_BooleanFalseIsPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	data := Defaults()
	data.Netbridge.Enabled = false

	require.NoError(t, SaveAt(path, data))

	got, _, err := ConfiguredAt(path)
	require.NoError(t, err)
	assert.False(t, got.Netbridge.Enabled)
}

func TestSaveAt_LeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := Defaults()
	data.Logger.Level = "debug"
	require.NoError(t, SaveAt(path, data))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "config.yaml", entries[0].Name())
}

func TestSaveAt_OverwritesPreviousOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	first := Defaults()
	first.Logger.Level = "debug"
	require.NoError(t, SaveAt(path, first))

	second := Defaults()
	second.Vault.TTL = "48h"
	require.NoError(t, SaveAt(path, second))

	got, _, err := ConfiguredAt(path)
	require.NoError(t, err)
	assert.Equal(t, Defaults().Logger.Level, got.Logger.Level)
	assert.Equal(t, "48h", got.Vault.TTL)
}

func TestSaveAt_EveryFieldRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	want := ConfigData{
		Netbridge: Netbridge{Enabled: false, EphemeralPortStart: 50000, EphemeralPortEnd: 50500},
		API:       API{Host: "tcp://127.0.0.1:9000"},
		Logger:    Logger{Enabled: false, Level: "debug"},
		Manifold:  Manifold{FetchTimeout: "45s"},
		Vault:     Vault{SweepInterval: "10m", TTL: "48h", IndexTTL: "360h"},
		Arrows:    Arrows{AutoRetry: ArrowAutoRetry{Enabled: false, Retries: 7}},
		Search:    Search{PerProviderLimit: 10, FetchConcurrency: 4, ProviderTimeout: "20s"},
	}

	require.NoError(t, SaveAt(path, want))

	got, corrected, err := ConfiguredAt(path)
	require.NoError(t, err)
	assert.Empty(t, corrected)
	assert.Equal(t, want, got)
}

func TestSaveAt_UnwritablePath_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	err := SaveAt(filepath.Join(blocker, "config.yaml"), Defaults())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config:")
}

func TestConfigured_UsesRealConfigPath(t *testing.T) {
	isolateHome(t)

	path := metadata.GetConfigPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))

	want := Defaults()
	want.Vault.TTL = "72h"
	require.NoError(t, Save(want))

	got, corrected, err := Configured()

	require.NoError(t, err)
	assert.Empty(t, corrected)
	assert.Equal(t, "72h", got.Vault.TTL)
}
