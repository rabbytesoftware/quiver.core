package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	core, shutdown := New()
	t.Cleanup(func() { _ = shutdown() })

	require.NotNil(t, core, "New() returned nil")
	assert.NotNil(t, core.metadata, "metadata is not initialized")
	assert.NotNil(t, core.config, "config is not initialized")
}

func TestCore_GetMetadata(t *testing.T) {
	core, shutdown := New()
	t.Cleanup(func() { _ = shutdown() })
	metadata := core.GetMetadata()

	require.NotNil(t, metadata, "GetMetadata() returned nil")
	assert.Same(t, metadata, core.GetMetadata(), "GetMetadata() should return the same instance")
}

func TestCore_GetConfig(t *testing.T) {
	core, shutdown := New()
	t.Cleanup(func() { _ = shutdown() })
	config := core.GetConfig()

	require.NotNil(t, config, "GetConfig() returned nil")
	assert.Same(t, config, core.GetConfig(), "GetConfig() should return the same instance")
}

func TestCoreStructure(t *testing.T) {
	core, shutdown := New()
	t.Cleanup(func() { _ = shutdown() })

	assert.NotNil(t, core.metadata, "Core.metadata field is nil")
	assert.NotNil(t, core.config, "Core.config field is nil")
}

func TestNewAt_ReturnsPopulatedCore(t *testing.T) {
	c, shutdown := NewAt(t.TempDir())
	t.Cleanup(func() { _ = shutdown() })

	require.NotNil(t, c, "NewAt() returned nil")
	assert.NotNil(t, c.metadata, "metadata is not initialized")
	assert.NotNil(t, c.config, "config is not initialized")
}

func TestNewAt_InvalidConfigField_LogsCorrectionWithoutPanicking(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("config:\n  vault:\n    ttl: banana\n"), 0o644))

	var shutdown func() error
	assert.NotPanics(t, func() { _, shutdown = NewAt(dir) })
	t.Cleanup(func() { _ = shutdown() })
}

func TestNewAt_ReadsConfigFromHomeDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`config:
  api:
    host: "tcp://core-newat-host:3333"
`), 0o644))

	c, shutdown := NewAt(dir)
	t.Cleanup(func() { _ = shutdown() })
	assert.Equal(t, "tcp://core-newat-host:3333", c.GetConfig().Config.API.Host)
}

func TestCoreInitialization(t *testing.T) {
	core1, shutdown1 := New()
	t.Cleanup(func() { _ = shutdown1() })
	core2, shutdown2 := New()
	t.Cleanup(func() { _ = shutdown2() })

	assert.NotSame(t, core1, core2, "New() should create new instances each time")
	assert.Same(t, core1.GetMetadata(), core2.GetMetadata(), "Metadata should be singleton across Core instances")
	assert.Same(t, core1.GetConfig(), core2.GetConfig(), "Config should be singleton across Core instances")
}
