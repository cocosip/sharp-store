package obs

import (
	"context"
	"testing"
)

func TestBackendValidatesConfig(t *testing.T) {
	backend := New()
	if err := backend.ValidateConfig(context.Background(), nil); err == nil {
		t.Fatal("ValidateConfig() error = nil")
	}
	if err := backend.ValidateConfig(context.Background(), map[string]string{EndpointKey: "https://obs.example.test", BucketKey: "archive", AccessKeyIDKey: "access", AccessKeySecretKey: "secret"}); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}
