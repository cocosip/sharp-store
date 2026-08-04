package static

import (
	"context"

	store "github.com/cocosip/sharp-store"
)

type Source struct {
	configs map[store.ContainerKey]store.ContainerConfig
}

func New(configs map[store.ContainerKey]store.ContainerConfig) store.ConfigSource {
	copy := make(map[store.ContainerKey]store.ContainerConfig, len(configs))
	for key, config := range configs {
		copy[key] = config.Clone()
	}
	return &Source{configs: copy}
}

func (s *Source) Load(
	_ context.Context,
	key store.ContainerKey,
	_ store.Scope,
) (store.ContainerConfig, error) {
	config, ok := s.configs[key]
	if !ok {
		return store.ContainerConfig{}, store.ErrContainerNotFound
	}
	return config.Clone(), nil
}
