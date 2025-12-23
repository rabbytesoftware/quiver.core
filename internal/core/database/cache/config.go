package cache

import (
	"time"
)

const (
	defaultTTL             = 5 * time.Minute
	defaultCleanupInterval = 1 * time.Minute
)

type CacheConfig struct {
	DefaultTTL      time.Duration `yaml:"default_ttl"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		DefaultTTL:      defaultTTL,
		CleanupInterval: defaultCleanupInterval,
	}
}

func (c CacheConfig) IsValid() bool {
	return (c.DefaultTTL > 0) && (c.CleanupInterval > 0)
}
