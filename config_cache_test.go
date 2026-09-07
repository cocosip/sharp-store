package store

import (
	"context"
	"testing"
	"time"
)

func TestMemoryConfigCacheIsolatesEntriesByTenantContext(t *testing.T) {
	t.Parallel()

	cache := NewMemoryConfigCache(MemoryConfigCacheOptions{TTL: time.Minute})
	tenantA := DefaultTenantContext{ID: "tenant-a", Code: "alpha"}
	tenantB := DefaultTenantContext{ID: "tenant-b", Code: "beta"}
	configA := ContainerConfig{Backend: "filesystem", Values: map[string]string{"root": "D:/a"}}
	configB := ContainerConfig{Backend: "filesystem", Values: map[string]string{"root": "D:/b"}}

	if err := cache.Set(context.Background(), "images", tenantA, configA); err != nil {
		t.Fatalf("Set(tenant-a) error = %v", err)
	}
	if err := cache.Set(context.Background(), "images", tenantB, configB); err != nil {
		t.Fatalf("Set(tenant-b) error = %v", err)
	}

	gotA, ok, err := cache.Get(context.Background(), "images", tenantA)
	if err != nil || !ok {
		t.Fatalf("Get(tenant-a) = (%#v, %t, %v), want cache hit", gotA, ok, err)
	}
	gotB, ok, err := cache.Get(context.Background(), "images", tenantB)
	if err != nil || !ok {
		t.Fatalf("Get(tenant-b) = (%#v, %t, %v), want cache hit", gotB, ok, err)
	}
	if gotA.Values["root"] != "D:/a" || gotB.Values["root"] != "D:/b" {
		t.Fatalf("cached roots = (%q, %q), want tenant-isolated values", gotA.Values["root"], gotB.Values["root"])
	}
}

func TestMemoryConfigCacheSnapshotsValuesAndExpiresEntries(t *testing.T) {
	now := time.Date(2026, time.September, 7, 10, 0, 0, 0, time.UTC)
	cache := newMemoryConfigCache(time.Minute, func() time.Time { return now })
	tenant := DefaultTenantContext{ID: "tenant-a"}
	config := ContainerConfig{Backend: "filesystem", Values: map[string]string{"root": "D:/original"}}

	if err := cache.Set(context.Background(), "images", tenant, config); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	config.Values["root"] = "D:/caller-mutated"

	first, ok, err := cache.Get(context.Background(), "images", tenant)
	if err != nil || !ok {
		t.Fatalf("Get() = (%#v, %t, %v), want cache hit", first, ok, err)
	}
	if got := first.Values["root"]; got != "D:/original" {
		t.Fatalf("cached root = %q, want snapshot", got)
	}
	first.Values["root"] = "D:/reader-mutated"

	second, ok, err := cache.Get(context.Background(), "images", tenant)
	if err != nil || !ok || second.Values["root"] != "D:/original" {
		t.Fatalf("second Get() = (%#v, %t, %v), want independent snapshot", second, ok, err)
	}

	now = now.Add(time.Minute)
	_, ok, err = cache.Get(context.Background(), "images", tenant)
	if err != nil {
		t.Fatalf("Get(expired) error = %v", err)
	}
	if ok {
		t.Fatal("Get(expired) hit = true, want false")
	}
}

func TestMemoryConfigCacheDeleteUsesTenantContext(t *testing.T) {
	t.Parallel()

	cache := NewMemoryConfigCache(MemoryConfigCacheOptions{TTL: time.Minute})
	tenant := externalTenantContext{id: "tenant-a"}
	if err := cache.Set(context.Background(), "images", tenant, ContainerConfig{Backend: "filesystem"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := cache.Delete(context.Background(), "images", tenant); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := cache.Get(context.Background(), "images", tenant); err != nil || ok {
		t.Fatalf("Get() after Delete = (_, %t, %v), want miss", ok, err)
	}
}

type externalTenantContext struct{ id string }

func (t externalTenantContext) TenantID() string { return t.id }
func (externalTenantContext) TenantCode() string { return "" }
func (externalTenantContext) TenantName() string { return "" }
