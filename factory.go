package store

import (
	"context"
	"errors"
	"io"
)

// Container is safe for concurrent use by multiple goroutines. Each operation
// has independent request state. Callers must not concurrently reuse mutable
// method inputs, such as an io.Reader or a destination path.
type Container interface {
	Configuration() ContainerConfig
	Save(ctx context.Context, fileID string, body io.Reader, extension string, overwrite bool) (string, error)
	Delete(ctx context.Context, fileID string) (bool, error)
	Exists(ctx context.Context, fileID string) (bool, error)
	Download(ctx context.Context, fileID, destination string) (bool, error)
	Get(ctx context.Context, fileID string) (io.ReadCloser, error)
	GetOrNil(ctx context.Context, fileID string) (io.ReadCloser, error)
	AccessURL(ctx context.Context, fileID string, options AccessURLOptions) (string, error)
}

type ContainerFactory interface {
	Open(ctx context.Context, key ContainerKey) (Container, error)
	OpenWithScope(ctx context.Context, key ContainerKey, scope Scope) (Container, error)
}

type Factory struct {
	configs  ConfigSource
	backends BackendResolver
	scopes   ScopeResolver
	names    NamingService
	keys     KeyBuilder
}

type FactoryOptions struct {
	Scopes ScopeResolver
	Names  NamingService
	Keys   KeyBuilder
}

func NewFactory(configs ConfigSource, backends BackendResolver) (ContainerFactory, error) {
	return NewFactoryWithOptions(configs, backends, FactoryOptions{})
}

func NewFactoryWithOptions(
	configs ConfigSource,
	backends BackendResolver,
	options FactoryOptions,
) (ContainerFactory, error) {
	if configs == nil {
		return nil, errors.New("config source is required")
	}
	if backends == nil {
		return nil, errors.New("backend resolver is required")
	}
	if options.Scopes == nil {
		options.Scopes = emptyScopeResolver{}
	}
	if options.Names == nil {
		options.Names = identityNamingService{}
	}
	if options.Keys == nil {
		options.Keys = defaultKeyBuilder{}
	}
	return &Factory{
		configs:  configs,
		backends: backends,
		scopes:   options.Scopes,
		names:    options.Names,
		keys:     options.Keys,
	}, nil
}

func (f *Factory) Open(ctx context.Context, key ContainerKey) (Container, error) {
	scope, err := f.scopes.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	return f.OpenWithScope(ctx, key, scope)
}

func (f *Factory) OpenWithScope(ctx context.Context, key ContainerKey, scope Scope) (Container, error) {
	config, err := f.configs.Load(ctx, key, scope)
	if err != nil {
		return nil, err
	}
	backend, err := f.backends.Resolve(config.Backend)
	if err != nil {
		return nil, err
	}
	if config.TenantMode == TenantShared {
		scope = scope.withoutTenant()
	}
	scope = scope.clone()
	return &container{
		key:     key,
		config:  config.Clone(),
		scope:   scope,
		backend: backend,
		names:   f.names,
		keys:    f.keys,
	}, nil
}
