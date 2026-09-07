package store_test

import (
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/aliyun"
	"github.com/cocosip/sharp-store/backend/aws"
	"github.com/cocosip/sharp-store/backend/azure"
	"github.com/cocosip/sharp-store/backend/filesystem"
	"github.com/cocosip/sharp-store/backend/ks3"
	"github.com/cocosip/sharp-store/backend/minio"
	"github.com/cocosip/sharp-store/backend/obs"
	"github.com/cocosip/sharp-store/backend/s3"
)

type externalTenant struct {
	id   string
	code string
	name string
}

func (t externalTenant) TenantID() string   { return t.id }
func (t externalTenant) TenantCode() string { return t.code }
func (t externalTenant) TenantName() string { return t.name }

func TestTenantContextSupportsBuiltInAndExternalImplementations(t *testing.T) {
	tests := []struct {
		name   string
		tenant store.TenantContext
		wantID string
	}{
		{
			name: "built-in",
			tenant: store.DefaultTenantContext{
				ID: "tenant-a", Code: "alpha", Name: "Alpha Hospital",
			},
			wantID: "tenant-a",
		},
		{name: "external adapter", tenant: externalTenant{id: "tenant-b"}, wantID: "tenant-b"},
		{name: "no tenant", tenant: store.NoTenant(), wantID: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.tenant.TenantID(); got != test.wantID {
				t.Fatalf("TenantID() = %q, want %q", got, test.wantID)
			}
		})
	}
}

func TestProviderConfigsExposeContainerSettings(t *testing.T) {
	tests := []store.BackendConfig{
		filesystem.Config{Root: "D:/files"}, s3.Config{Bucket: "bucket"}, aws.Config{Bucket: "bucket"},
		minio.Config{Bucket: "bucket", Endpoint: "minio:9000"}, ks3.Config{Bucket: "bucket", Endpoint: "ks3.example"},
		azure.Config{ConnectionString: "connection", Container: "bucket"}, aliyun.Config{Endpoint: "endpoint", Bucket: "bucket"},
		obs.Config{Endpoint: "endpoint", Bucket: "bucket"},
	}
	for _, provider := range tests {
		config := store.NewContainerConfig(provider)
		if config.Backend == "" || len(config.Values) == 0 {
			t.Fatalf("config %#v is not externally usable", provider)
		}
	}
}
