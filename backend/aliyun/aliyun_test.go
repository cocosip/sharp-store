package aliyun

import (
	"context"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestBackendValidatesConfig(t *testing.T) {
	backends := store.NewBackendRegistry(New())
	if err := backends.ValidateConfig(context.Background(), Name, nil); err == nil {
		t.Fatal("ValidateConfig() error = nil")
	}
	if err := backends.ValidateConfig(context.Background(), Name, map[string]string{EndpointKey: "https://oss.example.test", BucketKey: "archive", AccessKeyIDKey: "access", AccessKeySecretKey: "secret"}); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}
