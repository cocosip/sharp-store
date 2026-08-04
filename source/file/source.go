package file

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	store "github.com/cocosip/sharp-store"
)

type Decoder interface {
	Decode(reader io.Reader) (map[store.ContainerKey]store.ContainerConfig, error)
}

type JSONDecoder struct{}

func (JSONDecoder) Decode(reader io.Reader) (map[store.ContainerKey]store.ContainerConfig, error) {
	var document struct {
		Containers map[store.ContainerKey]store.ContainerConfig `json:"containers"`
	}
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return nil, err
	}
	return document.Containers, nil
}

type Source struct {
	path    string
	decoder Decoder

	mu      sync.RWMutex
	configs map[store.ContainerKey]store.ContainerConfig
}

func Open(path string, decoder Decoder) (*Source, error) {
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
