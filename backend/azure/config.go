package azure

type Config struct{ ConnectionString, Container string }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{ConnectionStringKey: c.ConnectionString, ContainerKey: c.Container}
}
