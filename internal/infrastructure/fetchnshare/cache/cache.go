package cache

import (
	"sync"
	"time"
)

type Item struct {
	Data       []byte
	Expiration int64
}

type Cache struct {
	items map[string]Item
	mu    sync.RWMutex
}

func New() *Cache {
	return &Cache{
		items: make(map[string]Item),
	}
}

func (c *Cache) Set(key string, data []byte, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = Item{
		Data:       data,
		Expiration: time.Now().Add(duration).UnixNano(),
	}
}

func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, found := c.items[key]
	if !found || time.Now().UnixNano() > item.Expiration {
		return nil, false
	}
	return item.Data, true
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]Item)
}

func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}
