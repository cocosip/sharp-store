package management

import (
	"context"
	"sync"

	store "github.com/cocosip/sharp-store"
)

type CacheKey struct {
	TenantID string
	Key      store.ContainerKey
}

type Cache interface {
	Get(ctx context.Context, key CacheKey) (Container, bool, error)
	Set(ctx context.Context, key CacheKey, container Container) error
	Delete(ctx context.Context, key CacheKey) error
}

type MemoryCache struct {
	mu      sync.RWMutex
	entries map[CacheKey]Container
}

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{entries: make(map[CacheKey]Container)}
}

func (c *MemoryCache) Get(ctx context.Context, key CacheKey) (Container, bool, error) {
	if err := ctx.Err(); err != nil {
		return Container{}, false, err
	}
	c.mu.RLock()
	container, ok := c.entries[key]
	c.mu.RUnlock()
	return container.Clone(), ok, nil
}

func (c *MemoryCache) Set(ctx context.Context, key CacheKey, container Container) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.entries[key] = container.Clone()
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Delete(ctx context.Context, key CacheKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
	return nil
}
