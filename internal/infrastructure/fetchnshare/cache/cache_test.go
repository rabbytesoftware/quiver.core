package cache

import (
	"testing"
	"time"
)

func TestCache_SetAndGet(t *testing.T) {
	c := New()
	key := "test-key"
	data := []byte("test data")
	ttl := 1 * time.Hour

	c.Set(key, data, ttl)

	got, found := c.Get(key)
	if !found {
		t.Error("Expected to find key in cache")
	}
	if string(got) != string(data) {
		t.Errorf("Got %q, want %q", string(got), string(data))
	}
}

func TestCache_GetExpired(t *testing.T) {
	c := New()
	key := "test-key"
	data := []byte("test data")
	ttl := 1 * time.Millisecond

	c.Set(key, data, ttl)
	time.Sleep(10 * time.Millisecond)

	_, found := c.Get(key)
	if found {
		t.Error("Expected expired key to not be found")
	}
}

func TestCache_GetNonexistent(t *testing.T) {
	c := New()

	_, found := c.Get("nonexistent")
	if found {
		t.Error("Expected nonexistent key to not be found")
	}
}

func TestCache_Delete(t *testing.T) {
	c := New()
	key := "test-key"
	data := []byte("test data")

	c.Set(key, data, 1*time.Hour)
	c.Delete(key)

	_, found := c.Get(key)
	if found {
		t.Error("Expected deleted key to not be found")
	}
}

func TestCache_Clear(t *testing.T) {
	c := New()

	c.Set("key1", []byte("data1"), 1*time.Hour)
	c.Set("key2", []byte("data2"), 1*time.Hour)

	if c.Size() != 2 {
		t.Errorf("Expected size 2, got %d", c.Size())
	}

	c.Clear()

	if c.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", c.Size())
	}
}

func TestCache_Size(t *testing.T) {
	c := New()

	if c.Size() != 0 {
		t.Errorf("Expected empty cache, got size %d", c.Size())
	}

	c.Set("key1", []byte("data1"), 1*time.Hour)
	if c.Size() != 1 {
		t.Errorf("Expected size 1, got %d", c.Size())
	}

	c.Set("key2", []byte("data2"), 1*time.Hour)
	if c.Size() != 2 {
		t.Errorf("Expected size 2, got %d", c.Size())
	}
}

func TestCache_Concurrent(t *testing.T) {
	c := New()
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			key := string(rune('a' + id))
			c.Set(key, []byte("data"), 1*time.Hour)
			c.Get(key)
			c.Delete(key)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
