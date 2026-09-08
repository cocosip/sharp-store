package store

import (
	"context"
	"testing"
)

func TestDefaultKeyBuilder_RejectsTraversal(t *testing.T) {
	for _, fileID := range []string{"../tenant-b/secret", `..\tenant-b\secret`, "folder/../../secret", "", "/", ".", "folder/../secret"} {
		t.Run(fileID, func(t *testing.T) {
			_, err := (defaultKeyBuilder{}).Build(context.Background(), FileRequest{FileID: fileID, Tenant: DefaultTenantContext{ID: "tenant-a"}})
			if err == nil {
				t.Fatal("Build() accepted invalid file ID")
			}
		})
	}
	for _, tenant := range []string{"..", ".", "a/b", `a\b`} {
		if _, err := (defaultKeyBuilder{}).Build(context.Background(), FileRequest{FileID: "file", Tenant: DefaultTenantContext{ID: tenant}}); err == nil {
			t.Errorf("Build() accepted tenant %q", tenant)
		}
	}
}
