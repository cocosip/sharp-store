package management

import (
	"context"

	store "github.com/cocosip/sharp-store"
)

type Service struct {
	repository Repository
	cache      Cache
	validator  store.ConfigValidator
}

func NewService(repository Repository, cache Cache) *Service {
	return NewServiceWithValidator(repository, cache, nil)
}

func NewServiceWithValidator(
	repository Repository,
	cache Cache,
	validator store.ConfigValidator,
) *Service {
	return &Service{repository: repository, cache: cache, validator: validator}
}

func (s *Service) Create(ctx context.Context, container Container) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scope := store.Scope{Tenant: store.Tenant{ID: container.TenantID}}
	_, exists, err := s.repository.Find(ctx, container.Key, scope)
	if err != nil {
		return err
	}
	if exists {
		return store.ErrContainerExists
	}
	if err := s.validate(ctx, container); err != nil {
		return err
	}
	if err := s.repository.Create(ctx, container); err != nil {
		return err
	}
	if s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, cacheKeyFor(container))
}

func (s *Service) Update(ctx context.Context, container Container) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	previous, ok, err := s.repository.Get(ctx, container.ID)
	if err != nil {
		return err
	}
	if !ok {
		return store.ErrContainerNotFound
	}
	if err := s.validate(ctx, container); err != nil {
		return err
	}
	if err := s.repository.Update(ctx, container); err != nil {
		return err
	}
	if s.cache == nil {
		return nil
	}
	if err := s.cache.Delete(ctx, cacheKeyFor(previous)); err != nil {
		return err
	}
	return s.cache.Delete(ctx, cacheKeyFor(container))
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	container, ok, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return store.ErrContainerNotFound
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	if s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, cacheKeyFor(container))
}

func cacheKeyFor(container Container) CacheKey {
	return CacheKey{TenantID: container.TenantID, Key: container.Key}
}

func (s *Service) validate(ctx context.Context, container Container) error {
	if s.validator == nil {
		return nil
	}
	return s.validator.ValidateConfig(ctx, container.Config.Backend, container.Config.Values)
}
