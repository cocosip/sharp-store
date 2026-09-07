package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/filesystem"
	filesource "github.com/cocosip/sharp-store/source/file"
	"github.com/cocosip/sharp-store/source/static"
)

func TestFactoryUsesDatabaseFreeConfigSources(t *testing.T) {
	tests := []struct {
		name   string
		source func(t *testing.T, root string) store.ConfigSource
	}{
		{
			name: "static",
			source: func(_ *testing.T, root string) store.ConfigSource {
				return static.New(map[store.ContainerKey]store.ContainerConfig{
					"documents": store.NewContainerConfig(filesystem.Config{Root: root}),
				})
			},
		},
		{
			name: "file",
			source: func(t *testing.T, root string) store.ConfigSource {
				path := filepath.Join(t.TempDir(), "storage.json")
				document := fmt.Sprintf(
					`{"containers":{"documents":{"backend":"filesystem","values":{"root":%q}}}}`,
					filepath.ToSlash(root),
				)
				if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
				source, err := filesource.Open(path, filesource.JSONDecoder{})
				if err != nil {
					t.Fatalf("file.Open() error = %v", err)
				}
				return source
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			factory, err := store.NewFactory(
				test.source(t, root),
				store.NewBackendRegistry(filesystem.New()),
			)
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			container, err := factory.Open(context.Background(), "documents")
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
		})
	}
}
