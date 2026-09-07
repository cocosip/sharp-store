package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFactoryOpenUsesConfigSourceAndDelegatesSave(t *testing.T) {
	t.Parallel()

	backend := newMemoryBackend("memory")
	factory, err := NewFactory(
		NewConfigOptions(staticConfigSource{configs: map[ContainerKey]ContainerConfig{
			"dicom": {Backend: "memory"},
		}}),
		NewContainerOptions(NewBackendRegistry(backend)),
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	container, err := factory.Open(
		context.Background(),
		"dicom",
		DefaultTenantContext{ID: "tenant-a", Code: "alpha"},
	)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if got := container.Configuration().Backend; got != "memory" {
		t.Fatalf("container backend = %q, want %q", got, "memory")
	}

	fileID, err := container.Save(
		context.Background(),
		"study/instance",
		bytes.NewBufferString("pixel-data"),
		".dcm",
		false,
	)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if fileID != "study/instance" {
		t.Fatalf("Save() file ID = %q, want %q", fileID, "study/instance")
	}

	if got := backend.content("alpha/study/instance"); got != "pixel-data" {
		t.Fatalf("backend content = %q, want %q", got, "pixel-data")
	}
}

func TestFactoryOpenAppliesTenantNamingAndKeyBuilder(t *testing.T) {
	t.Parallel()

	backend := newMemoryBackend("memory")
	factory, err := NewFactory(
		NewConfigOptions(staticConfigSource{configs: map[ContainerKey]ContainerConfig{
			"images": {Backend: "memory"},
		}}),
		NewContainerOptions(NewBackendRegistry(backend)).
			WithNamingService(lowercaseNamingService{}).
			WithKeyBuilder(containerAwareKeyBuilder{}),
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	container, err := factory.Open(
		context.Background(),
		"images",
		DefaultTenantContext{ID: "tenant-a", Code: "alpha"},
	)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	_, err = container.Save(
		context.Background(),
		"Study/INSTANCE",
		bytes.NewBufferString("thumbnail"),
		".jpg",
		false,
	)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if got := backend.content("alpha/images/study/instance"); got != "thumbnail" {
		t.Fatalf("backend content = %q, want %q", got, "thumbnail")
	}
}

func TestNamingServiceAppliesNormalizersInOrder(t *testing.T) {
	t.Parallel()

	service := NewNamingService(prefixLowercaseNormalizer{})
	name, err := service.Normalize(
		context.Background(),
		ContainerConfig{},
		"Images",
		"Study/INSTANCE",
	)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if name.Container != "images" {
		t.Fatalf("container = %q, want %q", name.Container, "images")
	}
	if name.FileID != "objects/study/instance" {
		t.Fatalf("file ID = %q, want %q", name.FileID, "objects/study/instance")
	}
}

type staticConfigSource struct {
	configs map[ContainerKey]ContainerConfig
}

type lowercaseNamingService struct{}

func (lowercaseNamingService) Normalize(
	_ context.Context,
	_ ContainerConfig,
	container ContainerKey,
	fileID string,
) (NormalizedName, error) {
	return NormalizedName{
		Container: ContainerKey(strings.ToLower(string(container))),
		FileID:    strings.ToLower(fileID),
	}, nil
}

type containerAwareKeyBuilder struct{}

func (containerAwareKeyBuilder) Build(_ context.Context, request FileRequest) (string, error) {
	return strings.Join([]string{request.Tenant.TenantCode(), string(request.Container), request.FileID}, "/"), nil
}

type prefixLowercaseNormalizer struct{}

func (prefixLowercaseNormalizer) NormalizeContainer(
	_ context.Context,
	_ ContainerConfig,
	container ContainerKey,
) (ContainerKey, error) {
	return ContainerKey(strings.ToLower(string(container))), nil
}

func (prefixLowercaseNormalizer) NormalizeFile(
	_ context.Context,
	_ ContainerConfig,
	fileID string,
) (string, error) {
	return "objects/" + strings.ToLower(fileID), nil
}

func (s staticConfigSource) Load(
	_ context.Context,
	key ContainerKey,
	_ TenantContext,
) (ContainerConfig, error) {
	config, ok := s.configs[key]
	if !ok {
		return ContainerConfig{}, ErrContainerNotFound
	}
	return config, nil
}

type memoryBackend struct {
	name    string
	objects map[string][]byte
}

func newMemoryBackend(name string) *memoryBackend {
	return &memoryBackend{name: name, objects: make(map[string][]byte)}
}

func (b *memoryBackend) Name() string {
	return b.name
}

func (b *memoryBackend) Save(_ context.Context, request SaveRequest) (string, error) {
	if !request.Overwrite {
		if _, ok := b.objects[request.Key]; ok {
			return "", ErrFileExists
		}
	}
	content, err := io.ReadAll(request.Body)
	if err != nil {
		return "", err
	}
	b.objects[request.Key] = content
	return request.FileID, nil
}

func (b *memoryBackend) Delete(_ context.Context, request FileRequest) (bool, error) {
	if _, ok := b.objects[request.Key]; !ok {
		return false, nil
	}
	delete(b.objects, request.Key)
	return true, nil
}

func (b *memoryBackend) Exists(_ context.Context, request FileRequest) (bool, error) {
	_, ok := b.objects[request.Key]
	return ok, nil
}

func (b *memoryBackend) Download(_ context.Context, _ DownloadRequest) (bool, error) {
	return false, errors.New("not implemented")
}

func (b *memoryBackend) GetOrNil(_ context.Context, request FileRequest) (io.ReadCloser, error) {
	content, ok := b.objects[request.Key]
	if !ok {
		return nil, nil
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (b *memoryBackend) AccessURL(_ context.Context, _ AccessURLRequest) (string, error) {
	return "", errors.New("not implemented")
}

func (b *memoryBackend) content(key string) string {
	return string(b.objects[key])
}
