package file

import (
	"context"
	"os"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestSourceReloadsJSONConfigurationByContainerKey(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/storage.json"
	if err := os.WriteFile(path, []byte(`{
  "containers": {
    "dicom": {
      "backend": "filesystem",
      "values": {"root": "D:/v1"}
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, err := Open(path, JSONDecoder{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	config, err := source.Load(context.Background(), "dicom", store.Scope{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := config.Values["root"]; got != "D:/v1" {
		t.Fatalf("root = %q, want %q", got, "D:/v1")
	}

	if err := os.WriteFile(path, []byte(`{
  "containers": {
    "dicom": {
      "backend": "filesystem",
      "values": {"root": "D:/v2"}
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile() update error = %v", err)
	}
	if err := source.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	config, err = source.Load(context.Background(), "dicom", store.Scope{})
	if err != nil {
		t.Fatalf("Load() after reload error = %v", err)
	}
	if got := config.Values["root"]; got != "D:/v2" {
		t.Fatalf("reloaded root = %q, want %q", got, "D:/v2")
	}
}
