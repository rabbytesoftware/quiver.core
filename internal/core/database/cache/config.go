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
	// Enabled determines if caching is active.
	Enabled bool `yaml:"enabled"`

	// DefaultTTL is the default time-to-live for cached entries.
	DefaultTTL time.Duration `yaml:"default_ttl"`

	// CleanupInterval is how often expired entries are cleaned up.
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
}

// DefaultCacheConfig returns a cache configuration with sensible defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:         true,
		DefaultTTL:      defaultTTL,
		CleanupInterval: defaultCleanupInterval,
	}
}

// IsValid checks if the cache configuration is valid.
func (c CacheConfig) IsValid() bool {
	if !c.Enabled {
		return true // Disabled cache is valid
	}
	return c.DefaultTTL > 0 && c.CleanupInterval > 0
}
