package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	core := New()

	require.NotNil(t, core, "New() returned nil")
	assert.NotNil(t, core.metadata, "metadata is not initialized")
	assert.NotNil(t, core.config, "config is not initialized")
}

func TestCore_GetMetadata(t *testing.T) {
	core := New()
	metadata := core.GetMetadata()

	require.NotNil(t, metadata, "GetMetadata() returned nil")
	assert.Same(t, metadata, core.GetMetadata(), "GetMetadata() should return the same instance")
}

func TestCore_GetConfig(t *testing.T) {
	core := New()
	config := core.GetConfig()

	require.NotNil(t, config, "GetConfig() returned nil")
	assert.Same(t, config, core.GetConfig(), "GetConfig() should return the same instance")
}

func TestCoreStructure(t *testing.T) {
	core := New()

	assert.NotNil(t, core.metadata, "Core.metadata field is nil")
	assert.NotNil(t, core.config, "Core.config field is nil")
}

func TestCoreInitialization(t *testing.T) {
	core1 := New()
	core2 := New()

	assert.NotSame(t, core1, core2, "New() should create new instances each time")
	assert.Same(t, core1.GetMetadata(), core2.GetMetadata(), "Metadata should be singleton across Core instances")
	assert.Same(t, core1.GetConfig(), core2.GetConfig(), "Config should be singleton across Core instances")
}
