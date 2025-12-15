package cache

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/config"
)

// CacheConfigFromYAML creates a CacheConfig from YAML configuration.
// It parses duration strings from the config and provides defaults if parsing fails.
func CacheConfigFromYAML() CacheConfig {
	yamlCache := config.GetCache()

	defaultTTL := 5 * time.Minute
	if yamlCache.DefaultTTL != "" {
		if parsed, err := time.ParseDuration(yamlCache.DefaultTTL); err == nil {
			defaultTTL = parsed
		}
	}

	cleanupInterval := 1 * time.Minute
	if yamlCache.CleanupInterval != "" {
		if parsed, err := time.ParseDuration(yamlCache.CleanupInterval); err == nil {
			cleanupInterval = parsed
		}
	}

	return CacheConfig{
		Enabled:         yamlCache.Enabled,
		DefaultTTL:      defaultTTL,
		CleanupInterval: cleanupInterval,
	}
}

