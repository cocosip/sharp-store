package gorm

import (
	"context"
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/management"
	"gorm.io/driver/sqlite"
	gormio "gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositoryCreatesAndFindsTenantContainer(t *testing.T) {
	t.Parallel()

	db, err := gormio.Open(sqlite.Open("file::memory:?cache=shared"), &gormio.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	options := Options{Table: "app_store_containers"}
	if err := db.Table(TableName(options)).AutoMigrate(&ContainerModel{}); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository := New(db, options)

	container := management.Container{
		ID:       "container-a",
		TenantID: "tenant-a",
		Key:      "dicom",
		Title:    "DICOM archive",
		Version:  3,
		Config: store.ContainerConfig{
			Backend: "filesystem",
			Values:  map[string]string{"root": "D:/archive"},
		},
	}
	if err := repository.Create(context.Background(), container); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	found, ok, err := repository.Find(
		context.Background(),
		"dicom",
		store.Scope{Tenant: store.Tenant{ID: "tenant-a"}},
	)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if !ok {
		t.Fatal("Find() found = false, want true")
	}
	if got := found.Config.Values["root"]; got != "D:/archive" {
		t.Fatalf("root = %q, want %q", got, "D:/archive")
	}

	_, ok, err = repository.Find(
		context.Background(),
		"dicom",
		store.Scope{Tenant: store.Tenant{ID: "tenant-b"}},
	)
	if err != nil {
		t.Fatalf("Find() other tenant error = %v", err)
	}
	if ok {
		t.Fatal("Find() found a different tenant's container")
	}
}
