package static

import (
	"context"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestSourceLoadReturnsIndependentConfigSnapshot(t *testing.T) {
	t.Parallel()

	source := New(map[store.ContainerKey]store.ContainerConfig{
		"archive": {
			Backend: "filesystem",
			Values:  map[string]string{"root": "D:/archive"},
		},
	})

	config, err := source.Load(context.Background(), "archive", store.Scope{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	config.Values["root"] = "D:/mutated"

	loadedAgain, err := source.Load(context.Background(), "archive", store.Scope{})
	if err != nil {
		t.Fatalf("Load() second error = %v", err)
	}
	if got := loadedAgain.Values["root"]; got != "D:/archive" {
		t.Fatalf("second root = %q, want %q", got, "D:/archive")
	}
}
