package azure

import (
	"fmt"
	"strconv"

	store "github.com/cocosip/sharp-store"
)

type Config struct {
	ConnectionString           string
	Container                  string
	CreateContainerIfNotExists bool
}

func NewConfig() Config { return Config{} }

func (c Config) WithConnectionString(connectionString string) Config {
	c.ConnectionString = connectionString
	return c
}
func (c Config) WithContainer(container string) Config { c.Container = container; return c }
func (c Config) WithCreateContainerIfNotExists(create bool) Config {
	c.CreateContainerIfNotExists = create
	return c
}

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{
		ConnectionStringKey: c.ConnectionString, ContainerKey: c.Container,
		CreateContainerIfNotExistsKey: strconv.FormatBool(c.CreateContainerIfNotExists),
	}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	create := false
	if raw := config.Values[CreateContainerIfNotExistsKey]; raw != "" {
		var err error
		create, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %s: %v", store.ErrInvalidConfig, CreateContainerIfNotExistsKey, err)
		}
	}
	return Config{ConnectionString: config.Values[ConnectionStringKey], Container: config.Values[ContainerKey], CreateContainerIfNotExists: create}, nil
}
