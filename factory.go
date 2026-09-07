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
	Open(ctx context.Context, key ContainerKey, tenant TenantContext) (Container, error)
}

type ContainerOptions struct {
	backends BackendResolver
	names    NamingService
	keys     KeyBuilder
}

func NewContainerOptions(backends BackendResolver) *ContainerOptions {
	return &ContainerOptions{backends: backends}
}

func (o *ContainerOptions) WithNamingService(names NamingService) *ContainerOptions {
	if o != nil {
		o.names = names
	}
	return o
}

func (o *ContainerOptions) WithKeyBuilder(keys KeyBuilder) *ContainerOptions {
	if o != nil {
		o.keys = keys
	}
	return o
}

type Factory struct {
	configs  *cachedConfigSource
	backends BackendResolver
	names    NamingService
	keys     KeyBuilder
}

func NewFactory(configs *ConfigOptions, containers *ContainerOptions) (*Factory, error) {
	if configs == nil || configs.source == nil {
		return nil, errors.New("config source is required")
	}
	cache := configs.cache
	if cache == nil {
		cache = NewMemoryConfigCache()
	}
	return newFactory(newCachedConfigSource(configs.source, cache), containers)
}

func newFactory(configs *cachedConfigSource, options *ContainerOptions) (*Factory, error) {
	if configs == nil {
		return nil, errors.New("config source is required")
	}
	if options == nil || options.backends == nil {
		return nil, errors.New("backend resolver is required")
	}
	names := options.names
	if names == nil {
		names = identityNamingService{}
	}
	keys := options.keys
	if keys == nil {
		keys = defaultKeyBuilder{}
	}
	return &Factory{configs: configs, backends: options.backends, names: names, keys: keys}, nil
}

func (f *Factory) Open(
	ctx context.Context,
	key ContainerKey,
	tenant TenantContext,
) (Container, error) {
	tenant = snapshotTenant(tenant)
	config, err := f.configs.Load(ctx, key, tenant)
	if err != nil {
		return nil, err
	}
	backend, err := f.backends.Resolve(config.Backend)
	if err != nil {
		return nil, err
	}
	return &container{
		key:     key,
		config:  config.Clone(),
		tenant:  tenant,
		backend: backend,
		names:   f.names,
		keys:    f.keys,
	}, nil
}
