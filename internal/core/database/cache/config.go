package cache

import (
	"time"
)

const (
	defaultTTL             = 5 * time.Minute
	defaultCleanupInterval = 1 * time.Minute
)

// CacheConfig holds configuration for the cache layer.
type CacheConfig struct {
	Enabled         bool          `yaml:"enabled"`          // Determines if caching is active.
	DefaultTTL      time.Duration `yaml:"default_ttl"`      // Default time-to-live for cached entries.
	CleanupInterval time.Duration `yaml:"cleanup_interval"` // How often expired entries are cleaned up.
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:         true,
		DefaultTTL:      defaultTTL,
		CleanupInterval: defaultCleanupInterval,
	}
}

func (c CacheConfig) IsValid() bool {
	if !c.Enabled {
		return true
	}
	return c.DefaultTTL > 0 && c.CleanupInterval > 0
}
