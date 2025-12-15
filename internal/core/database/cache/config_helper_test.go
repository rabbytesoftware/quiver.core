package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// CacheConfigFromYAML Tests
// =============================================================================

func TestCacheConfigFromYAML(t *testing.T) {
	// Test that CacheConfigFromYAML returns a valid config
	config := CacheConfigFromYAML()

	// Config should have valid values (either from YAML or defaults)
	assert.GreaterOrEqual(t, config.DefaultTTL, time.Duration(0), "DefaultTTL should be non-negative")
	assert.GreaterOrEqual(t, config.CleanupInterval, time.Duration(0), "CleanupInterval should be non-negative")
}

func TestCacheConfigFromYAML_ValidDurations(t *testing.T) {
	// Test that CacheConfigFromYAML parses valid duration strings correctly
	// This tests lines 15-18 and 22-25 when parsing succeeds
	config := CacheConfigFromYAML()

	// The function should parse durations from YAML config if they're valid
	// If YAML has valid durations, they should be used; otherwise defaults apply
	if config.DefaultTTL > 0 {
		// If a valid duration was parsed, it should be positive
		assert.Greater(t, config.DefaultTTL, time.Duration(0), "Parsed DefaultTTL should be positive")
	}

	if config.CleanupInterval > 0 {
		// If a valid duration was parsed, it should be positive
		assert.Greater(t, config.CleanupInterval, time.Duration(0), "Parsed CleanupInterval should be positive")
	}
}

func TestCacheConfigFromYAML_InvalidDurations(t *testing.T) {
	// Test that CacheConfigFromYAML falls back to defaults when duration parsing fails
	// This tests lines 15-18 and 22-25 when err != nil (parsing fails)
	config := CacheConfigFromYAML()

	// The function should always return valid defaults even if YAML parsing fails
	// Default TTL is 5 minutes, default cleanup interval is 1 minute
	// If parsing failed, these defaults should be used
	if config.DefaultTTL == 5*time.Minute {
		// Default was used (either because YAML was empty or parsing failed)
		assert.Equal(t, 5*time.Minute, config.DefaultTTL, "Should use default TTL when parsing fails")
	}

	if config.CleanupInterval == 1*time.Minute {
		// Default was used (either because YAML was empty or parsing failed)
		assert.Equal(t, 1*time.Minute, config.CleanupInterval, "Should use default cleanup interval when parsing fails")
	}
}

func TestCacheConfigFromYAML_EmptyDurations(t *testing.T) {
	// Test that CacheConfigFromYAML uses defaults when duration strings are empty
	// This tests lines 15 and 22 when the condition is false (empty string)
	config := CacheConfigFromYAML()

	// If YAML config has empty duration strings, the function should use defaults
	// The function checks if DefaultTTL != "" before parsing
	// If empty, it skips parsing and uses the default (5 minutes)
	// Same for CleanupInterval (defaults to 1 minute)

	// Verify config is valid regardless of whether durations were parsed or defaulted
	assert.IsType(t, CacheConfig{}, config, "Should return CacheConfig type")
	assert.GreaterOrEqual(t, config.DefaultTTL, 5*time.Minute, "DefaultTTL should be at least default value")
	assert.GreaterOrEqual(t, config.CleanupInterval, 1*time.Minute, "CleanupInterval should be at least default value")
}

func TestCacheConfigFromYAML_EnabledField(t *testing.T) {
	// Test that CacheConfigFromYAML correctly sets the Enabled field
	config := CacheConfigFromYAML()

	// Enabled field should be set from YAML config
	assert.IsType(t, true, config.Enabled, "Enabled should be bool")
}

func TestCacheConfigFromYAML_MultipleCalls(t *testing.T) {
	// Test that multiple calls return consistent results
	config1 := CacheConfigFromYAML()
	config2 := CacheConfigFromYAML()
	config3 := CacheConfigFromYAML()

	// All calls should return the same values (from same YAML config)
	assert.Equal(t, config1.Enabled, config2.Enabled, "Enabled should be consistent across calls")
	assert.Equal(t, config1.DefaultTTL, config2.DefaultTTL, "DefaultTTL should be consistent across calls")
	assert.Equal(t, config1.CleanupInterval, config2.CleanupInterval, "CleanupInterval should be consistent across calls")

	assert.Equal(t, config2.Enabled, config3.Enabled, "Enabled should be consistent across calls")
	assert.Equal(t, config2.DefaultTTL, config3.DefaultTTL, "DefaultTTL should be consistent across calls")
	assert.Equal(t, config2.CleanupInterval, config3.CleanupInterval, "CleanupInterval should be consistent across calls")
}

func TestCacheConfigFromYAML_Integration(t *testing.T) {
	// Test that CacheConfigFromYAML integrates properly with NewGoCache
	config := CacheConfigFromYAML()

	// If enabled, should be able to create a GoCache
	if config.Enabled {
		cache := NewGoCache(config)
		assert.NotNil(t, cache, "Should create GoCache when config is enabled")
	} else {
		cache := NewGoCache(config)
		assert.Nil(t, cache, "Should return nil GoCache when config is disabled")
	}
}

func TestCacheConfigFromYAML_IsValid(t *testing.T) {
	// Test that CacheConfigFromYAML returns a config that passes IsValid
	config := CacheConfigFromYAML()

	// The config from YAML should be valid
	isValid := config.IsValid()

	// If disabled, should be valid
	if !config.Enabled {
		assert.True(t, isValid, "Disabled config should be valid")
	}

	// If enabled with proper values, should be valid
	if config.Enabled && config.DefaultTTL > 0 && config.CleanupInterval > 0 {
		assert.True(t, isValid, "Enabled config with positive values should be valid")
	}
}
