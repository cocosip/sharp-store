package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestDocumentationExamplesLoadAllSupportedBackends(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		decoder Decoder
	}{
		{name: "JSON", path: "storage.example.json", decoder: JSONDecoder{}},
		{name: "YAML", path: "storage.example.yaml", decoder: YAMLDecoder{}},
		{name: "TOML", path: "storage.example.toml", decoder: TOMLDecoder{}},
	}
	want := map[store.ContainerKey]string{
		"filesystem": "filesystem", "s3": "s3", "aws": "aws", "minio": "minio",
		"ks3": "ks3", "azure": "azure", "aliyun": "aliyun", "obs": "obs",
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := Open(filepath.Join("..", "..", "docs", test.path), test.decoder)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			for key, backend := range want {
				config, err := source.Load(context.Background(), key, store.Scope{})
				if err != nil {
					t.Fatalf("Load(%q) error = %v", key, err)
				}
				if config.Backend != backend {
					t.Errorf("Load(%q).Backend = %q, want %q", key, config.Backend, backend)
				}
			}
		})
	}
}

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

func TestSourceLoadsYAMLConfigurationByContainerKey(t *testing.T) {
	path := t.TempDir() + "/storage.yaml"
	if err := os.WriteFile(path, []byte(`containers:
  dicom:
    backend: filesystem
    tenant_mode: 1
    values:
      root: D:/yaml
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, err := Open(path, YAMLDecoder{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	config, err := source.Load(context.Background(), "dicom", store.Scope{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.TenantMode != store.TenantShared {
		t.Fatalf("TenantMode = %d, want %d", config.TenantMode, store.TenantShared)
	}
	if got := config.Values["root"]; got != "D:/yaml" {
		t.Fatalf("root = %q, want %q", got, "D:/yaml")
	}
}

func TestSourceLoadsTOMLConfigurationByContainerKey(t *testing.T) {
	path := t.TempDir() + "/storage.toml"
	if err := os.WriteFile(path, []byte(`[containers.dicom]
backend = "filesystem"
tenant_mode = 1

[containers.dicom.values]
root = "D:/toml"
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, err := Open(path, TOMLDecoder{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	config, err := source.Load(context.Background(), "dicom", store.Scope{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.TenantMode != store.TenantShared {
		t.Fatalf("TenantMode = %d, want %d", config.TenantMode, store.TenantShared)
	}
	if got := config.Values["root"]; got != "D:/toml" {
		t.Fatalf("root = %q, want %q", got, "D:/toml")
	}
}

func TestSourceReloadRejectsInvalidYAMLWithoutReplacingSnapshot(t *testing.T) {
	path := t.TempDir() + "/storage.yaml"
	if err := os.WriteFile(path, []byte(`containers:
  dicom:
    backend: filesystem
    values:
      root: D:/stable
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, err := Open(path, YAMLDecoder{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("containers: ["), 0o600); err != nil {
		t.Fatalf("WriteFile() invalid update error = %v", err)
	}

	if err := source.Reload(context.Background()); err == nil {
		t.Fatal("Reload() error = nil, want invalid YAML error")
	}
	config, err := source.Load(context.Background(), "dicom", store.Scope{})
	if err != nil {
		t.Fatalf("Load() after rejected reload error = %v", err)
	}
	if got := config.Values["root"]; got != "D:/stable" {
		t.Fatalf("root after rejected reload = %q, want %q", got, "D:/stable")
	}
}
