package store

import (
	"context"
	"sync"
	"time"
)

const DefaultConfigCacheTTL = 5 * time.Minute

// ConfigCache caches resolved container configuration. Cache implementations
// receive the same TenantContext as ConfigSource implementations.
type ConfigCache interface {
	Get(ctx context.Context, key ContainerKey, tenant TenantContext) (ContainerConfig, bool, error)
	Set(ctx context.Context, key ContainerKey, tenant TenantContext, config ContainerConfig) error
	Delete(ctx context.Context, key ContainerKey, tenant TenantContext) error
}

type MemoryConfigCacheOptions struct {
	TTL time.Duration
}

type memoryConfigCacheKey struct {
	tenantID string
	key      ContainerKey
}

type memoryConfigCacheEntry struct {
	config    ContainerConfig
	expiresAt time.Time
}

type MemoryConfigCache struct {
	mu      sync.RWMutex
	entries map[memoryConfigCacheKey]memoryConfigCacheEntry
	ttl     time.Duration
	now     func() time.Time
}

func NewMemoryConfigCache(options ...MemoryConfigCacheOptions) ConfigCache {
	ttl := DefaultConfigCacheTTL
	if len(options) != 0 && options[0].TTL > 0 {
		ttl = options[0].TTL
	}
	return newMemoryConfigCache(ttl, time.Now)
}

func newMemoryConfigCache(ttl time.Duration, now func() time.Time) *MemoryConfigCache {
	return &MemoryConfigCache{
		entries: make(map[memoryConfigCacheKey]memoryConfigCacheEntry),
		ttl:     ttl,
		now:     now,
	}
}

func (c *MemoryConfigCache) Get(
	ctx context.Context,
	key ContainerKey,
	tenant TenantContext,
) (ContainerConfig, bool, error) {
	if err := ctx.Err(); err != nil {
		return ContainerConfig{}, false, err
	}
	cacheKey := newMemoryConfigCacheKey(key, tenant)
	c.mu.RLock()
	entry, ok := c.entries[cacheKey]
	c.mu.RUnlock()
	if !ok {
		return ContainerConfig{}, false, nil
	}
	if !c.now().Before(entry.expiresAt) {
		c.mu.Lock()
		if current, exists := c.entries[cacheKey]; exists && !c.now().Before(current.expiresAt) {
			delete(c.entries, cacheKey)
		}
		c.mu.Unlock()
		return ContainerConfig{}, false, nil
	}
	return entry.config.Clone(), true, nil
}

func (c *MemoryConfigCache) Set(
	ctx context.Context,
	key ContainerKey,
	tenant TenantContext,
	config ContainerConfig,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entry := memoryConfigCacheEntry{
		config:    config.Clone(),
		expiresAt: c.now().Add(c.ttl),
	}
	c.mu.Lock()
	c.entries[newMemoryConfigCacheKey(key, tenant)] = entry
	c.mu.Unlock()
	return nil
}

func (c *MemoryConfigCache) Delete(
	ctx context.Context,
	key ContainerKey,
	tenant TenantContext,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.entries, newMemoryConfigCacheKey(key, tenant))
	c.mu.Unlock()
	return nil
}

func newMemoryConfigCacheKey(key ContainerKey, tenant TenantContext) memoryConfigCacheKey {
	tenantID := ""
	if tenant != nil {
		tenantID = tenant.TenantID()
	}
	return memoryConfigCacheKey{tenantID: tenantID, key: key}
}
