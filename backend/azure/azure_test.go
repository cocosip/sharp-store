package azure

import (
	"context"
	"testing"
)

func TestBackendValidatesAzureBlobConfiguration(t *testing.T) {
	t.Parallel()

	backend := New()
	if err := backend.ValidateConfig(context.Background(), map[string]string{}); err == nil {
		t.Fatal("ValidateConfig() error = nil, want missing connection string and container")
	}
	if err := backend.ValidateConfig(context.Background(), map[string]string{
		ConnectionStringKey: "DefaultEndpointsProtocol=https;AccountName=account;AccountKey=key",
		ContainerKey:        "archive",
	}); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}
