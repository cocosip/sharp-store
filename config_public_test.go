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
