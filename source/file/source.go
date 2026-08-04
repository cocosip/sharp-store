package file

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	store "github.com/cocosip/sharp-store"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type Decoder interface {
	Decode(reader io.Reader) (map[store.ContainerKey]store.ContainerConfig, error)
}

type ReloadableSource interface {
	store.ConfigSource
	Reload(ctx context.Context) error
}

type JSONDecoder struct{}

func (JSONDecoder) Decode(reader io.Reader) (map[store.ContainerKey]store.ContainerConfig, error) {
	var document configDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return nil, err
	}
	return document.configs(), nil
}

// YAMLDecoder decodes a YAML storage configuration document.
type YAMLDecoder struct{}

func (YAMLDecoder) Decode(reader io.Reader) (map[store.ContainerKey]store.ContainerConfig, error) {
	var document configDocument
	if err := yaml.NewDecoder(reader).Decode(&document); err != nil {
		return nil, err
	}
	return document.configs(), nil
}

// TOMLDecoder decodes a TOML storage configuration document.
type TOMLDecoder struct{}

func (TOMLDecoder) Decode(reader io.Reader) (map[store.ContainerKey]store.ContainerConfig, error) {
	var document configDocument
	if err := toml.NewDecoder(reader).Decode(&document); err != nil {
		return nil, err
	}
	return document.configs(), nil
}

type configDocument struct {
	Containers map[string]containerDocument `json:"containers" yaml:"containers" toml:"containers"`
}

type containerDocument struct {
	Backend    string            `json:"backend" yaml:"backend" toml:"backend"`
	TenantMode store.TenantMode  `json:"tenantMode" yaml:"tenant_mode" toml:"tenant_mode"`
	Values     map[string]string `json:"values" yaml:"values" toml:"values"`
}

func (d configDocument) configs() map[store.ContainerKey]store.ContainerConfig {
	configs := make(map[store.ContainerKey]store.ContainerConfig, len(d.Containers))
	for key, config := range d.Containers {
		configs[store.ContainerKey(key)] = store.ContainerConfig{
			Backend:    config.Backend,
			TenantMode: config.TenantMode,
			Values:     config.Values,
		}
	}
	return configs
}

type Source struct {
	path    string
	decoder Decoder

	mu      sync.RWMutex
	configs map[store.ContainerKey]store.ContainerConfig
}

func Open(path string, decoder Decoder) (ReloadableSource, error) {
	if decoder == nil {
		return nil, errors.New("configuration decoder is required")
	}
	source := &Source{path: path, decoder: decoder}
	if err := source.Reload(context.Background()); err != nil {
		return nil, err
	}
	return source, nil
}

func (s *Source) Load(
	ctx context.Context,
	key store.ContainerKey,
	_ store.Scope,
) (store.ContainerConfig, error) {
	if err := ctx.Err(); err != nil {
		return store.ContainerConfig{}, err
	}
	s.mu.RLock()
	config, ok := s.configs[key]
	s.mu.RUnlock()
	if !ok {
		return store.ContainerConfig{}, store.ErrContainerNotFound
	}
	return config.Clone(), nil
}

func (s *Source) Reload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	configs, err := s.decoder.Decode(file)
	if err != nil {
		return err
	}

	snapshot := make(map[store.ContainerKey]store.ContainerConfig, len(configs))
	for key, config := range configs {
		snapshot[key] = config.Clone()
	}
	s.mu.Lock()
	s.configs = snapshot
	s.mu.Unlock()
	return nil
}
