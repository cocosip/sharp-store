package filesystem

type Config struct{ Root, BaseURL string }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{RootKey: c.Root, BaseURLKey: c.BaseURL}
}
