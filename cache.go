package mirage

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// Cache is an interface for caching query results. Implementations can use
// in-memory, Redis, Memcached, or any other backend.
type Cache interface {
	// Get retrieves a cached value by key. Returns true if found.
	// dest must be a pointer to the type that was stored.
	Get(ctx context.Context, key string, dest any) (bool, error)

	// Set stores a value with a TTL. Use 0 for no expiration.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete removes a cached value by key.
	Delete(ctx context.Context, key string) error

	// Invalidate removes all cached values whose keys start with prefix.
	Invalidate(ctx context.Context, prefix string) error
}

type cacheItem struct {
	value   any
	expires time.Time
}

// InMemoryCache is a goroutine-safe in-memory Cache implementation.
// Suitable for single-instance applications and development. For production
// with multiple instances, use a distributed cache implementing the Cache
// interface (e.g. Redis, Memcached).
type InMemoryCache struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
}

// NewInMemoryCache creates a new InMemoryCache.
func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{items: make(map[string]*cacheItem)}
}

func (c *InMemoryCache) Get(_ context.Context, key string, dest any) (bool, error) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return false, nil
	}

	if !item.expires.IsZero() && time.Now().After(item.expires) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return false, nil
	}

	srcBytes, err := json.Marshal(item.value)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(srcBytes, dest)
}

func (c *InMemoryCache) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	var expires time.Time
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}

	c.mu.Lock()
	c.items[key] = &cacheItem{value: value, expires: expires}
	c.mu.Unlock()
	return nil
}

func (c *InMemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
	return nil
}

func (c *InMemoryCache) Invalidate(_ context.Context, prefix string) error {
	c.mu.Lock()
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			delete(c.items, key)
		}
	}
	c.mu.Unlock()
	return nil
}
