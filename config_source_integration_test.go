package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/filesystem"
	"github.com/cocosip/sharp-store/source/static"
)

func TestFactoryUsesBuiltInStaticConfigSource(t *testing.T) {
	root := t.TempDir()
	configs := static.New(map[store.ContainerKey]store.ContainerConfig{
		"documents": store.NewContainerConfig(filesystem.Config{Root: root}),
	})
	factory, err := store.NewFactory(
		store.NewConfigOptions(configs),
		store.NewContainerOptions(store.NewBackendRegistry(filesystem.New())),
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	container, err := factory.Open(context.Background(), "documents", store.NoTenant())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = container.Save(
		context.Background(),
		"report.txt",
		strings.NewReader("content"),
		"",
		false,
	)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "report.txt"))
	if err != nil {
		t.Fatalf("ReadFile(saved object) error = %v", err)
	}
	if string(content) != "content" {
		t.Fatalf("saved content = %q, want %q", content, "content")
	}
}
