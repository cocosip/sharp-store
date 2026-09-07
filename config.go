package store

import (
	"context"
	"errors"
)

var (
	ErrContainerNotFound = errors.New("container configuration not found")
	ErrContainerExists   = errors.New("container configuration already exists")
	ErrBackendNotFound   = errors.New("storage backend not found")
	ErrFileNotFound      = errors.New("file not found")
	ErrFileExists        = errors.New("file already exists")
	ErrInvalidConfig     = errors.New("invalid storage configuration")
	ErrUnsupported       = errors.New("operation is not supported")
)

// ContainerKey identifies a logical storage container by name. Tenant identity
// is passed separately through TenantContext.
type ContainerKey string

type TenantMode uint8

const (
	TenantScoped TenantMode = iota
	TenantShared
)

type ContainerConfig struct {
	Backend    string
	TenantMode TenantMode
	Values     map[string]string
}

// BackendConfig is a provider-owned, typed configuration that can be passed to
// any ConfigSource after conversion to ContainerConfig.
type BackendConfig interface {
	BackendName() string
	Values() map[string]string
}

func NewContainerConfig(config BackendConfig) ContainerConfig {
	if config == nil {
		return ContainerConfig{}
	}
	values := config.Values()
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return ContainerConfig{Backend: config.BackendName(), Values: copy}
}

func (c ContainerConfig) WithTenantMode(mode TenantMode) ContainerConfig {
	c.TenantMode = mode
	return c
}

func (c ContainerConfig) Clone() ContainerConfig {
	copy := c
	if c.Values != nil {
		copy.Values = make(map[string]string, len(c.Values))
		for key, value := range c.Values {
			copy.Values[key] = value
		}
	}
	return copy
}

type ConfigSource interface {
	Load(ctx context.Context, key ContainerKey, tenant TenantContext) (ContainerConfig, error)
}
