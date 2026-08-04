package management

import (
	"context"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestSourceUsesTenantScopedCacheAndInvalidation(t *testing.T) {
	t.Parallel()

	repository := &testRepository{records: map[cacheKey]Container{
		{tenantID: "tenant-a", key: "dicom"}: {
			ID:       "container-a",
			TenantID: "tenant-a",
			Key:      "dicom",
			Config: store.ContainerConfig{
				Backend: "filesystem",
				Values:  map[string]string{"root": "D:/v1"},
			},
		},
	}}
	source := NewSource(repository, NewMemoryCache())
	scope := store.Scope{Tenant: store.Tenant{ID: "tenant-a"}}

	first, err := source.Load(context.Background(), "dicom", scope)
	if err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	if got := first.Values["root"]; got != "D:/v1" {
		t.Fatalf("first root = %q, want %q", got, "D:/v1")
	}

	repository.records[cacheKey{tenantID: "tenant-a", key: "dicom"}] = Container{
		ID:       "container-a",
		TenantID: "tenant-a",
		Key:      "dicom",
		Config: store.ContainerConfig{
			Backend: "filesystem",
			Values:  map[string]string{"root": "D:/v2"},
		},
	}

	cached, err := source.Load(context.Background(), "dicom", scope)
	if err != nil {
		t.Fatalf("cached Load() error = %v", err)
	}
	if got := cached.Values["root"]; got != "D:/v1" {
		t.Fatalf("cached root = %q, want %q", got, "D:/v1")
	}

	if err := source.Invalidate(context.Background(), "dicom", scope); err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}
	updated, err := source.Load(context.Background(), "dicom", scope)
	if err != nil {
		t.Fatalf("updated Load() error = %v", err)
	}
	if got := updated.Values["root"]; got != "D:/v2" {
		t.Fatalf("updated root = %q, want %q", got, "D:/v2")
	}
}

type cacheKey struct {
	tenantID string
	key      store.ContainerKey
}

type testRepository struct {
	records map[cacheKey]Container
}

func (r *testRepository) Find(
	_ context.Context,
	key store.ContainerKey,
	scope store.Scope,
) (Container, bool, error) {
	container, ok := r.records[cacheKey{tenantID: scope.Tenant.ID, key: key}]
	return container, ok, nil
}
