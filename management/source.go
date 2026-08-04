package management

import (
	"context"

	store "github.com/cocosip/sharp-store"
)

type Source struct {
	repository Reader
	cache      Cache
}

type InvalidatableSource interface {
	store.ConfigSource
	Invalidate(ctx context.Context, key store.ContainerKey, scope store.Scope) error
}

func NewSource(repository Reader, cache Cache) InvalidatableSource {
	return &Source{repository: repository, cache: cache}
}

func (s *Source) Load(
	ctx context.Context,
	key store.ContainerKey,
	scope store.Scope,
) (store.ContainerConfig, error) {
	if err := ctx.Err(); err != nil {
		return store.ContainerConfig{}, err
	}
	cacheKey := CacheKey{TenantID: scope.Tenant.ID, Key: key}
	if s.cache != nil {
		container, ok, err := s.cache.Get(ctx, cacheKey)
		if err != nil {
			return store.ContainerConfig{}, err
		}
		if ok {
			return container.Config.Clone(), nil
		}
	}

	container, ok, err := s.repository.Find(ctx, key, scope)
	if err != nil {
		return store.ContainerConfig{}, err
	}
	if !ok {
		return store.ContainerConfig{}, store.ErrContainerNotFound
	}
	if s.cache != nil {
		if err := s.cache.Set(ctx, cacheKey, container); err != nil {
			return store.ContainerConfig{}, err
		}
	}
	return container.Config.Clone(), nil
}

func (s *Source) Invalidate(ctx context.Context, key store.ContainerKey, scope store.Scope) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, CacheKey{TenantID: scope.Tenant.ID, Key: key})
}
