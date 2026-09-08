package store

import (
	"context"
	"strconv"
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

// VersionedConfigCache enables automatic cache filling without overwriting
// concurrent application updates. The version is opaque and must change on
// Set and Delete, including deletion of an absent entry. GetWithVersion must
// read the value and version atomically. SetIfVersion must atomically compare
// the version and write only if it is unchanged, returning false on conflict.
// Implementations may use a cache-wide version, conservatively rejecting fills
// after unrelated mutations. Versions must not be reused while fills can run.
// Caches implementing only ConfigCache are read by the factory but not filled.
type VersionedConfigCache interface {
	ConfigCache
	GetWithVersion(ctx context.Context, key ContainerKey, tenant TenantContext) (ContainerConfig, bool, string, error)
	SetIfVersion(ctx context.Context, key ContainerKey, tenant TenantContext, config ContainerConfig, version string) (bool, error)
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
	version uint64
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
	config, ok, _, err := c.GetWithVersion(ctx, key, tenant)
	return config, ok, err
}

func (c *MemoryConfigCache) GetWithVersion(
	ctx context.Context,
	key ContainerKey,
	tenant TenantContext,
) (ContainerConfig, bool, string, error) {
	if err := ctx.Err(); err != nil {
		return ContainerConfig{}, false, "", err
	}
	cacheKey := newMemoryConfigCacheKey(key, tenant)
	c.mu.RLock()
	entry, ok := c.entries[cacheKey]
	version := strconv.FormatUint(c.version, 10)
	c.mu.RUnlock()
	if !ok {
		return ContainerConfig{}, false, version, nil
	}
	if !c.now().Before(entry.expiresAt) {
		c.mu.Lock()
		if current, exists := c.entries[cacheKey]; exists && !c.now().Before(current.expiresAt) {
			delete(c.entries, cacheKey)
		}
		c.mu.Unlock()
		return ContainerConfig{}, false, version, nil
	}
	return entry.config.Clone(), true, version, nil
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
	c.version++
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
	c.version++
	c.mu.Unlock()
	return nil
}

func (c *MemoryConfigCache) SetIfVersion(
	ctx context.Context,
	key ContainerKey,
	tenant TenantContext,
	config ContainerConfig,
	version string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	entry := memoryConfigCacheEntry{config: config.Clone(), expiresAt: c.now().Add(c.ttl)}
	cacheKey := newMemoryConfigCacheKey(key, tenant)
	c.mu.Lock()
	defer c.mu.Unlock()
	if strconv.FormatUint(c.version, 10) != version {
		return false, nil
	}
	c.entries[cacheKey] = entry
	c.version++
	return true, nil
}

func newMemoryConfigCacheKey(key ContainerKey, tenant TenantContext) memoryConfigCacheKey {
	tenantID := ""
	if tenant != nil {
		tenantID = tenant.TenantID()
	}
	return memoryConfigCacheKey{tenantID: tenantID, key: key}
}
