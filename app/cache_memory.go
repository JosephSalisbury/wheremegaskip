package app

import (
	"context"
	"sync"
	"time"
)

// MemoryCache implements Cache using in-memory storage
type MemoryCache struct {
	data      map[string]memoryCacheEntry
	mu        sync.RWMutex
}

type memoryCacheEntry struct {
	locations []SkipLocation
	expiresAt time.Time
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		data: make(map[string]memoryCacheEntry),
	}
}

// Get retrieves data from the memory cache
func (c *MemoryCache) Get(ctx context.Context, key string) ([]SkipLocation, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[key]
	if !ok {
		return nil, nil // Cache miss
	}

	if time.Now().After(entry.expiresAt) {
		return nil, nil // Expired
	}

	return entry.locations, nil
}

// Set stores data in the memory cache with the given TTL
func (c *MemoryCache) Set(ctx context.Context, key string, data []SkipLocation, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = memoryCacheEntry{
		locations: data,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}
