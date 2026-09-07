package management_test

import (
	"context"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/management"
)

type memoryRepository struct {
	containers map[string]management.Container
}

func newMemoryRepository(containers ...management.Container) *memoryRepository {
	repository := &memoryRepository{containers: make(map[string]management.Container)}
	for _, container := range containers {
		repository.containers[container.ID] = container.Clone()
	}
	return repository
}

func (r *memoryRepository) Find(
	_ context.Context,
	key store.ContainerKey,
	scope store.Scope,
) (management.Container, bool, error) {
	for _, container := range r.containers {
		if container.TenantID == scope.Tenant.ID && container.Key == key {
			return container.Clone(), true, nil
		}
	}
	return management.Container{}, false, nil
}

func (r *memoryRepository) Get(
	_ context.Context,
	id string,
) (management.Container, bool, error) {
	container, ok := r.containers[id]
	return container.Clone(), ok, nil
}

func (r *memoryRepository) Create(_ context.Context, container management.Container) error {
	r.containers[container.ID] = container.Clone()
	return nil
}

func (r *memoryRepository) Update(_ context.Context, container management.Container) error {
	r.containers[container.ID] = container.Clone()
	return nil
}

func (r *memoryRepository) Delete(_ context.Context, id string) error {
	delete(r.containers, id)
	return nil
}

var _ management.Repository = (*memoryRepository)(nil)
