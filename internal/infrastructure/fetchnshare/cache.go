package fetchnshare

import (
	"sync"
	"time"
)

// Item in cache has data and expiration time
type Item struct {
	Data       []byte
	Expiration int64
}

// Cache struct with map to hold items and mutex for concurrency control
type Cache struct {
	items map[string]Item
	mu    sync.RWMutex
}

// NewCache creates a new Cache instance
func NewCache() *Cache {
	return &Cache{
		items: make(map[string]Item),
	}
}

// Set adds an item to the cache with a specified duration
func (c *Cache) Set(key string, data []byte, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = Item{
		Data:       data,
		Expiration: time.Now().Add(duration).UnixNano(),
	}
}

// Get retrieves an item from the cache if it exists and is not expired
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, found := c.items[key]
	if !found || time.Now().UnixNano() > item.Expiration {
		return nil, false
	}
	return item.Data, true
}

// Delete removes an item from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]Item)
}
