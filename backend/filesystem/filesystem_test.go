package filesystem

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestBackendSaveAndGetUseConfiguredRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	backend := New()
	request := store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{"root": root}},
			FileID: "instance",
			Key:    "tenant-a/studies/instance.dcm",
		},
		Body:      bytes.NewBufferString("pixel-data"),
		Extension: ".dcm",
	}

	fileID, err := backend.Save(context.Background(), request)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if fileID != "instance" {
		t.Fatalf("Save() file ID = %q, want %q", fileID, "instance")
	}

	path := filepath.Join(root, "tenant-a", "studies", "instance.dcm")
	if content, err := os.ReadFile(path); err != nil || string(content) != "pixel-data" {
		t.Fatalf("saved content = %q, error = %v", content, err)
	}

	reader, err := backend.GetOrNil(context.Background(), request.FileRequest)
	if err != nil {
		t.Fatalf("GetOrNil() error = %v", err)
	}
	defer func() { _ = reader.Close() }()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(content) != "pixel-data" {
		t.Fatalf("GetOrNil() content = %q, want %q", content, "pixel-data")
	}
}
