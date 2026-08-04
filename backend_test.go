package store

import (
	"context"
	"errors"
	"testing"
)

func TestBackendRegistryDescribesAndValidatesConfiguredBackend(t *testing.T) {
	t.Parallel()

	registry := NewBackendRegistry(configuredBackend{memoryBackend: newMemoryBackend("configured")})
	backends := registry.List()
	if len(backends) != 1 {
		t.Fatalf("List() count = %d, want 1", len(backends))
	}
	if backends[0].Name != "configured" {
		t.Fatalf("List() backend name = %q, want %q", backends[0].Name, "configured")
	}
	if len(backends[0].Options) != 1 || backends[0].Options[0].Name != "root" {
		t.Fatalf("List() options = %#v, want root option", backends[0].Options)
	}

	err := registry.ValidateConfig(context.Background(), "configured", nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ValidateConfig() error = %v, want ErrInvalidConfig", err)
	}
}

func TestNewContainerConfigUsesBackendConfig(t *testing.T) {
	config := NewContainerConfig(testBackendConfig{values: map[string]string{"root": "D:/archive"}})
	if config.Backend != "test" {
		t.Fatalf("Backend = %q, want test", config.Backend)
	}
	if config.Values["root"] != "D:/archive" {
		t.Fatalf("root = %q", config.Values["root"])
	}
}

type testBackendConfig struct{ values map[string]string }

func (testBackendConfig) BackendName() string         { return "test" }
func (c testBackendConfig) Values() map[string]string { return c.values }

type configuredBackend struct {
	*memoryBackend
}

func (configuredBackend) ConfigOptions() []ConfigOption {
	return []ConfigOption{{Name: "root", Type: "string", Required: true}}
}

func (configuredBackend) ValidateConfig(context.Context, map[string]string) error {
	return errors.New("root is required")
}
