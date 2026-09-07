package store

import (
	"context"
	"sync"
	"testing"
)

func TestFactoryOpenUsesSameAPIWithDefaultAndCustomCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*ConfigOptions)
	}{
		{name: "default memory cache"},
		{
			name: "custom cache",
			configure: func(options *ConfigOptions) {
				options.WithCache(NewMemoryConfigCache())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			backend := newMemoryBackend("memory")
			source := &countingConfigSource{config: ContainerConfig{Backend: "memory"}}
			configOptions := NewConfigOptions(source)
			if test.configure != nil {
				test.configure(configOptions)
			}
			factory, err := NewFactory(
				configOptions,
				NewContainerOptions(NewBackendRegistry(backend)),
			)
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			tenant := externalTenantContext{id: "tenant-a"}

			if _, err := factory.Open(context.Background(), "images", tenant); err != nil {
				t.Fatalf("first Open() error = %v", err)
			}
			if _, err := factory.Open(context.Background(), "images", tenant); err != nil {
				t.Fatalf("second Open() error = %v", err)
			}
			if got := source.callCount(); got != 1 {
				t.Fatalf("ConfigSource.Load() calls = %d, want 1", got)
			}
		})
	}
}

func TestFactoryConfigCacheIsolatesTenantsUsingTenantContext(t *testing.T) {
	t.Parallel()

	backend := newMemoryBackend("memory")
	source := &tenantConfigSource{calls: make(map[string]int)}
	factory, err := NewFactory(
		NewConfigOptions(source),
		NewContainerOptions(NewBackendRegistry(backend)),
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	for range 2 {
		if _, err := factory.Open(context.Background(), "images", DefaultTenantContext{ID: "tenant-a"}); err != nil {
			t.Fatalf("Open(tenant-a) error = %v", err)
		}
		if _, err := factory.Open(context.Background(), "images", DefaultTenantContext{ID: "tenant-b"}); err != nil {
			t.Fatalf("Open(tenant-b) error = %v", err)
		}
	}
	if got := source.callsFor("tenant-a"); got != 1 {
		t.Fatalf("tenant-a source calls = %d, want 1", got)
	}
	if got := source.callsFor("tenant-b"); got != 1 {
		t.Fatalf("tenant-b source calls = %d, want 1", got)
	}
}

func TestFactoryCoalescesConcurrentConfigCacheMisses(t *testing.T) {
	t.Parallel()

	backend := newMemoryBackend("memory")
	source := &blockingConfigSource{
		config:  ContainerConfig{Backend: "memory"},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	factory, err := NewFactory(
		NewConfigOptions(source),
		NewContainerOptions(NewBackendRegistry(backend)),
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	const workers = 12
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := factory.Open(
				context.Background(),
				"images",
				DefaultTenantContext{ID: "tenant-a"},
			)
			errs <- err
		}()
	}

	<-source.started
	close(source.release)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
	}
	if got := source.callCount(); got != 1 {
		t.Fatalf("ConfigSource.Load() calls = %d, want 1", got)
	}
}

type countingConfigSource struct {
	mu     sync.Mutex
	config ContainerConfig
	calls  int
}

func (s *countingConfigSource) Load(
	_ context.Context,
	_ ContainerKey,
	_ TenantContext,
) (ContainerConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.config.Clone(), nil
}

func (s *countingConfigSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type tenantConfigSource struct {
	mu    sync.Mutex
	calls map[string]int
}

func (s *tenantConfigSource) Load(
	_ context.Context,
	_ ContainerKey,
	tenant TenantContext,
) (ContainerConfig, error) {
	s.mu.Lock()
	s.calls[tenant.TenantID()]++
	s.mu.Unlock()
	return ContainerConfig{Backend: "memory"}, nil
}

func (s *tenantConfigSource) callsFor(tenantID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[tenantID]
}

type blockingConfigSource struct {
	mu      sync.Mutex
	config  ContainerConfig
	calls   int
	started chan struct{}
	release chan struct{}
}

func (s *blockingConfigSource) Load(
	_ context.Context,
	_ ContainerKey,
	_ TenantContext,
) (ContainerConfig, error) {
	s.mu.Lock()
	s.calls++
	if s.calls == 1 {
		close(s.started)
	}
	s.mu.Unlock()
	<-s.release
	return s.config.Clone(), nil
}

func (s *blockingConfigSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
