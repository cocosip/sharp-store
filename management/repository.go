package management

import (
	"context"

	store "github.com/cocosip/sharp-store"
)

type Reader interface {
	Find(ctx context.Context, key store.ContainerKey, scope store.Scope) (Container, bool, error)
}

type Repository interface {
	Reader
	Get(ctx context.Context, id string) (Container, bool, error)
	Create(ctx context.Context, container Container) error
	Update(ctx context.Context, container Container) error
	Delete(ctx context.Context, id string) error
}
