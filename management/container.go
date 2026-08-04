package management

import store "github.com/cocosip/sharp-store"

type Container struct {
	ID       string
	TenantID string
	Key      store.ContainerKey
	Title    string
	Config   store.ContainerConfig
	Version  uint64
}

func (c Container) Clone() Container {
	c.Config = c.Config.Clone()
	return c
}
