package store

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"
)

type FileRequest struct {
	Container ContainerKey
	Config    ContainerConfig
	Scope     Scope
	FileID    string
	Key       string
}

type SaveRequest struct {
	FileRequest
	Body      io.Reader
	Extension string
	Overwrite bool
}

type DownloadRequest struct {
	FileRequest
	Destination string
}

type AccessURLRequest struct {
	FileRequest
	ExpiresAt   *time.Time
	CheckExists bool
}

type Backend interface {
	Name() string
	Save(ctx context.Context, request SaveRequest) (string, error)
	Delete(ctx context.Context, request FileRequest) (bool, error)
	Exists(ctx context.Context, request FileRequest) (bool, error)
	Download(ctx context.Context, request DownloadRequest) (bool, error)
	GetOrNil(ctx context.Context, request FileRequest) (io.ReadCloser, error)
	AccessURL(ctx context.Context, request AccessURLRequest) (string, error)
}

type BackendResolver interface {
	Resolve(name string) (Backend, error)
}

type ConfigOption struct {
	Name        string
	Type        string
	Required    bool
	Example     string
	Description string
	Sensitive   bool
}

type BackendInfo struct {
	Name    string
	Options []ConfigOption
}

type BackendDescriptor interface {
	ConfigOptions() []ConfigOption
}

type BackendConfigValidator interface {
	ValidateConfig(ctx context.Context, values map[string]string) error
}

type ConfigValidator interface {
	ValidateConfig(ctx context.Context, name string, values map[string]string) error
}

type BackendCatalog interface {
	BackendResolver
	ConfigValidator
	List() []BackendInfo
}

type BackendRegistry struct {
	backends map[string]Backend
}

func NewBackendRegistry(backends ...Backend) BackendCatalog {
	registry := &BackendRegistry{backends: make(map[string]Backend, len(backends))}
	for _, backend := range backends {
		if backend != nil {
			registry.backends[backend.Name()] = backend
		}
	}
	return registry
}

func (r *BackendRegistry) Resolve(name string) (Backend, error) {
	backend, ok := r.backends[name]
	if !ok {
		return nil, ErrBackendNotFound
	}
	return backend, nil
}

func (r *BackendRegistry) List() []BackendInfo {
	backends := make([]BackendInfo, 0, len(r.backends))
	for name, backend := range r.backends {
		info := BackendInfo{Name: name}
		if descriptor, ok := backend.(BackendDescriptor); ok {
			info.Options = append([]ConfigOption(nil), descriptor.ConfigOptions()...)
		}
		backends = append(backends, info)
	}
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Name < backends[j].Name
	})
	return backends
}

func (r *BackendRegistry) ValidateConfig(
	ctx context.Context,
	name string,
	values map[string]string,
) error {
	backend, err := r.Resolve(name)
	if err != nil {
		return err
	}
	validator, ok := backend.(BackendConfigValidator)
	if !ok {
		return nil
	}
	if err := validator.ValidateConfig(ctx, values); err != nil {
		return fmt.Errorf("%w: backend %q: %v", ErrInvalidConfig, name, err)
	}
	return nil
}
