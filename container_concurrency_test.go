package store_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/filesystem"
)

func TestContainerSnapshotsScopeWhenOpened(t *testing.T) {
	root := t.TempDir()
	scope := store.Scope{Values: map[string]string{"prefix": "opened"}}
	container := openFilesystemContainer(t, root, scope)

	// A container must not retain the caller-owned map after it is opened.
	scope.Values["prefix"] = "changed"

	if _, err := container.Save(
		context.Background(),
		"instance.dcm",
		bytes.NewBufferString("pixel-data"),
		".dcm",
		false,
	); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "opened", "instance.dcm")); err != nil {
		t.Fatalf("object stored with opened scope = %v", err)
	}
}

func TestContainerSupportsConcurrentOperations(t *testing.T) {
	root := t.TempDir()
	container := openFilesystemContainer(t, root, store.Scope{Values: map[string]string{"prefix": "objects"}})

	const workers = 16
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func() {
			defer group.Done()

			fileID := fmt.Sprintf("instances/%02d.dcm", index)
			content := fmt.Sprintf("pixel-data-%02d", index)
			if _, err := container.Save(context.Background(), fileID, bytes.NewBufferString(content), ".dcm", false); err != nil {
				errs <- fmt.Errorf("Save(%q): %w", fileID, err)
				return
			}

			reader, err := container.Get(context.Background(), fileID)
			if err != nil {
				errs <- fmt.Errorf("Get(%q): %w", fileID, err)
				return
			}
			data, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil || string(data) != content {
				errs <- fmt.Errorf("Get(%q) content = %q, read error = %v, close error = %v", fileID, data, readErr, closeErr)
				return
			}

			destination := filepath.Join(root, "downloads", fmt.Sprintf("%02d.dcm", index))
			downloaded, err := container.Download(context.Background(), fileID, destination)
			if err != nil || !downloaded {
				errs <- fmt.Errorf("Download(%q) = (%t, %v)", fileID, downloaded, err)
				return
			}
			downloadedData, err := os.ReadFile(destination)
			if err != nil || string(downloadedData) != content {
				errs <- fmt.Errorf("downloaded %q content = %q, error = %v", fileID, downloadedData, err)
				return
			}

			exists, err := container.Exists(context.Background(), fileID)
			if err != nil || !exists {
				errs <- fmt.Errorf("Exists(%q) = (%t, %v)", fileID, exists, err)
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func openFilesystemContainer(t *testing.T, root string, scope store.Scope) store.Container {
	t.Helper()

	factory, err := store.NewFactoryWithOptions(
		testConfigSource{config: store.NewContainerConfig(filesystem.Config{Root: root})},
		store.NewBackendRegistry(filesystem.New()),
		store.FactoryOptions{Keys: scopeValueKeyBuilder{}},
	)
	if err != nil {
		t.Fatalf("NewFactoryWithOptions() error = %v", err)
	}

	container, err := factory.OpenWithScope(context.Background(), "files", scope)
	if err != nil {
		t.Fatalf("OpenWithScope() error = %v", err)
	}
	return container
}

type testConfigSource struct {
	config store.ContainerConfig
}

func (s testConfigSource) Load(
	context.Context,
	store.ContainerKey,
	store.Scope,
) (store.ContainerConfig, error) {
	return s.config, nil
}

type scopeValueKeyBuilder struct{}

func (scopeValueKeyBuilder) Build(_ context.Context, request store.FileRequest) (string, error) {
	return request.Scope.Values["prefix"] + "/" + request.FileID, nil
}
