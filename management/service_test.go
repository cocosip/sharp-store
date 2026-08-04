package management_test

import (
	"context"
	"errors"
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/filesystem"
	"github.com/cocosip/sharp-store/management"
	managementgorm "github.com/cocosip/sharp-store/management/gorm"
	"gorm.io/driver/sqlite"
	gormio "gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestServiceUpdateInvalidatesCachedContainerConfig(t *testing.T) {
	t.Parallel()

	db, err := gormio.Open(sqlite.Open("file:management-update?mode=memory&cache=shared"), &gormio.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := managementgorm.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repository := managementgorm.New(db)
	cache := management.NewMemoryCache()
	container := management.Container{
		ID:       "container-update",
		TenantID: "tenant-a",
		Key:      "dicom",
		Config: store.ContainerConfig{
			Backend: "filesystem",
			Values:  map[string]string{"root": "D:/v1"},
		},
	}
	if err := repository.Create(context.Background(), container); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	source := management.NewSource(repository, cache)
	scope := store.Scope{Tenant: store.Tenant{ID: "tenant-a"}}
	if _, err := source.Load(context.Background(), "dicom", scope); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	container.Config.Values["root"] = "D:/v2"
	service := management.NewService(repository, cache)
	if err := service.Update(context.Background(), container); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	config, err := source.Load(context.Background(), "dicom", scope)
	if err != nil {
		t.Fatalf("Load() after update error = %v", err)
	}
	if got := config.Values["root"]; got != "D:/v2" {
		t.Fatalf("root after update = %q, want %q", got, "D:/v2")
	}
}

func TestServiceDeleteInvalidatesCachedContainerConfig(t *testing.T) {
	t.Parallel()

	db, err := gormio.Open(sqlite.Open("file:management-delete?mode=memory&cache=shared"), &gormio.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := managementgorm.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repository := managementgorm.New(db)
	cache := management.NewMemoryCache()
	container := management.Container{
		ID:       "container-delete",
		TenantID: "tenant-a",
		Key:      "thumbnails",
		Config: store.ContainerConfig{
			Backend: "filesystem",
			Values:  map[string]string{"root": "D:/thumbnails"},
		},
	}
	if err := repository.Create(context.Background(), container); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	source := management.NewSource(repository, cache)
	scope := store.Scope{Tenant: store.Tenant{ID: "tenant-a"}}
	if _, err := source.Load(context.Background(), "thumbnails", scope); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	service := management.NewService(repository, cache)
	if err := service.Delete(context.Background(), container.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = source.Load(context.Background(), "thumbnails", scope)
	if !errors.Is(err, store.ErrContainerNotFound) {
		t.Fatalf("Load() after delete error = %v, want ErrContainerNotFound", err)
	}
}

func TestServiceCreateRejectsDuplicateKeyWithinTenant(t *testing.T) {
	t.Parallel()

	db, err := gormio.Open(sqlite.Open("file:management-create?mode=memory&cache=shared"), &gormio.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := managementgorm.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	service := management.NewService(managementgorm.New(db), management.NewMemoryCache())
	first := management.Container{
		ID:       "container-first",
		TenantID: "tenant-a",
		Key:      "uploads",
		Config:   store.ContainerConfig{Backend: "filesystem"},
	}
	if err := service.Create(context.Background(), first); err != nil {
		t.Fatalf("Create() first error = %v", err)
	}

	duplicate := first
	duplicate.ID = "container-duplicate"
	err = service.Create(context.Background(), duplicate)
	if !errors.Is(err, store.ErrContainerExists) {
		t.Fatalf("Create() duplicate error = %v, want ErrContainerExists", err)
	}
}

func TestServiceCreateValidatesBackendConfigBeforePersisting(t *testing.T) {
	t.Parallel()

	db, err := gormio.Open(sqlite.Open("file:management-validation?mode=memory&cache=shared"), &gormio.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := managementgorm.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	service := management.NewServiceWithValidator(
		managementgorm.New(db),
		management.NewMemoryCache(),
		store.NewBackendRegistry(filesystem.New()),
	)
	err = service.Create(context.Background(), management.Container{
		ID:       "container-invalid",
		TenantID: "tenant-a",
		Key:      "uploads",
		Config:   store.ContainerConfig{Backend: filesystem.Name},
	})
	if !errors.Is(err, store.ErrInvalidConfig) {
		t.Fatalf("Create() error = %v, want ErrInvalidConfig", err)
	}
}
