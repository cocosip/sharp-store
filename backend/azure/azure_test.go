package azure

import (
	"context"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestBackendValidatesAzureBlobConfiguration(t *testing.T) {
	t.Parallel()

	backends := store.NewBackendRegistry(New())
	if err := backends.ValidateConfig(context.Background(), Name, map[string]string{}); err == nil {
		t.Fatal("ValidateConfig() error = nil, want missing connection string and container")
	}
	if err := backends.ValidateConfig(context.Background(), Name, map[string]string{
		ConnectionStringKey: "DefaultEndpointsProtocol=https;AccountName=account;AccountKey=key",
		ContainerKey:        "archive",
	}); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}
