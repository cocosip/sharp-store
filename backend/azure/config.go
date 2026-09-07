package azure

import (
	"fmt"

	store "github.com/cocosip/sharp-store"
)

type Config struct{ ConnectionString, Container string }

func NewConfig() Config { return Config{} }

func (c Config) WithConnectionString(connectionString string) Config {
	c.ConnectionString = connectionString
	return c
}
func (c Config) WithContainer(container string) Config { c.Container = container; return c }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{ConnectionStringKey: c.ConnectionString, ContainerKey: c.Container}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	return Config{ConnectionString: config.Values[ConnectionStringKey], Container: config.Values[ContainerKey]}, nil
}
