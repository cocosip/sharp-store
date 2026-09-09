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

func TestContainerSnapshotsTenantWhenOpened(t *testing.T) {
	root := t.TempDir()
	tenant := &mutableTenantContext{id: "tenant-a", code: "opened", name: "Opened"}
	container := openFilesystemContainer(t, root, tenant)

	// A container must not retain the caller-owned tenant after it is opened.
	tenant.code = "changed"

	if _, err := container.Save(
		context.Background(),
		"instance.dcm",
		bytes.NewBufferString("pixel-data"),
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
	container := openFilesystemContainer(t, root, store.DefaultTenantContext{ID: "tenant-a", Code: "objects"})

	const workers = 16
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func() {
			defer group.Done()

			fileID := fmt.Sprintf("instances/%02d.dcm", index)
			content := fmt.Sprintf("pixel-data-%02d", index)
			if _, err := container.Save(context.Background(), fileID, bytes.NewBufferString(content), false); err != nil {
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

func openFilesystemContainer(t *testing.T, root string, tenant store.TenantContext) store.Container {
	t.Helper()

	factory, err := store.NewFactory(
		store.NewConfigOptions(testConfigSource{config: store.NewContainerConfig(filesystem.Config{Root: root})}),
		store.NewContainerOptions(store.NewBackendRegistry(filesystem.New())).WithKeyBuilder(tenantCodeKeyBuilder{}),
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	container, err := factory.Open(context.Background(), "files", tenant)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return container
}

type testConfigSource struct {
	config store.ContainerConfig
}

func (s testConfigSource) Load(
	context.Context,
	store.ContainerKey,
	store.TenantContext,
) (store.ContainerConfig, error) {
	return s.config, nil
}

type tenantCodeKeyBuilder struct{}

func (tenantCodeKeyBuilder) Build(_ context.Context, request store.FileRequest) (string, error) {
	return request.Tenant.TenantCode() + "/" + request.FileID, nil
}

type mutableTenantContext struct {
	id   string
	code string
	name string
}

func (t *mutableTenantContext) TenantID() string   { return t.id }
func (t *mutableTenantContext) TenantCode() string { return t.code }
func (t *mutableTenantContext) TenantName() string { return t.name }
