package cache

import "time"

const (
	NumCounters       = 1e7
	MaxCost           = 100
	BufferItems       = 64
	DefaultTTL        = 5 * time.Minute
	DefaultCostOfItem = 1
)

type CacheConfig struct {
	NumCounters       int64
	MaxCost           int64
	BufferItems       int64
	DefaultTTL        time.Duration
	DefaultCostOfItem int64
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		NumCounters:       NumCounters,
		MaxCost:           MaxCost,
		BufferItems:       BufferItems,
		DefaultTTL:        DefaultTTL,
		DefaultCostOfItem: DefaultCostOfItem,
	}
}

func (c CacheConfig) IsValid() bool {
	return (c.NumCounters > 0) && (c.MaxCost > 0) && (c.BufferItems > 0) && (c.DefaultTTL > 0) && (c.DefaultCostOfItem > 0)
}
