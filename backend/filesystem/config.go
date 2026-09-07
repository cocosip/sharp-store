package filesystem

import (
	"fmt"

	store "github.com/cocosip/sharp-store"
)

type Config struct{ Root, BaseURL string }

func NewConfig() Config { return Config{} }

func (c Config) WithRoot(root string) Config       { c.Root = root; return c }
func (c Config) WithBaseURL(baseURL string) Config { c.BaseURL = baseURL; return c }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{RootKey: c.Root, BaseURLKey: c.BaseURL}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	return Config{Root: config.Values[RootKey], BaseURL: config.Values[BaseURLKey]}, nil
}
