package store

import (
	"context"
	"sync"
)

type ConfigOptions struct {
	source ConfigSource
	cache  ConfigCache
}

func NewConfigOptions(source ConfigSource) *ConfigOptions {
	return &ConfigOptions{source: source}
}

func (o *ConfigOptions) WithCache(cache ConfigCache) *ConfigOptions {
	if o != nil {
		o.cache = cache
	}
	return o
}

type configLoadKey struct {
	tenantID string
	key      ContainerKey
}

type configLoadCall struct {
	done   chan struct{}
	config ContainerConfig
	err    error
}

type cachedConfigSource struct {
	source ConfigSource
	cache  ConfigCache

	mu    sync.Mutex
	loads map[configLoadKey]*configLoadCall
}

func newCachedConfigSource(source ConfigSource, cache ConfigCache) *cachedConfigSource {
	return &cachedConfigSource{source: source, cache: cache, loads: make(map[configLoadKey]*configLoadCall)}
}

func (s *cachedConfigSource) Load(
	ctx context.Context,
	key ContainerKey,
	tenant TenantContext,
) (ContainerConfig, error) {
	// A failing cache read is treated as a miss so a cache outage cannot take
	// container opens down while the source is healthy.
	config, ok, err := s.cache.Get(ctx, key, tenant)
	if err == nil && ok {
		return config, nil
	}

	loadKey := configLoadKey{key: key}
	if tenant != nil {
		loadKey.tenantID = tenant.TenantID()
	}
	s.mu.Lock()
	if call, exists := s.loads[loadKey]; exists {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ContainerConfig{}, ctx.Err()
		case <-call.done:
			return call.config.Clone(), call.err
		}
	}
	call := &configLoadCall{done: make(chan struct{})}
	s.loads[loadKey] = call
	s.mu.Unlock()

	versioned, canFill := s.cache.(VersionedConfigCache)
	var version string
	if canFill {
		call.config, ok, version, call.err = versioned.GetWithVersion(ctx, key, tenant)
	} else {
		call.config, ok, call.err = s.cache.Get(ctx, key, tenant)
	}
	if call.err != nil {
		call.config, ok, version, call.err = ContainerConfig{}, false, "", nil
	}
	if !ok {
		call.config, call.err = s.source.Load(ctx, key, tenant)
		if call.err == nil {
			call.config = call.config.Clone()
			if canFill {
				// Best effort: a failing fill must not fail the load.
				_, _ = versioned.SetIfVersion(ctx, key, tenant, call.config, version)
			}
		}
	}

	s.mu.Lock()
	delete(s.loads, loadKey)
	close(call.done)
	s.mu.Unlock()
	return call.config.Clone(), call.err
}
