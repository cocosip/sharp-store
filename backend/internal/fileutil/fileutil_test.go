package fileutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileCreatesParentsAndWritesContent(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "nested", "dir", "object.txt")

	copied, err := WriteFile(destination, strings.NewReader("content"))
	if err != nil || !copied {
		t.Fatalf("WriteFile() = (%t, %v), want (true, nil)", copied, err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "content" {
		t.Fatalf("written content = %q, error = %v", data, err)
	}
}

func TestWriteFileReturnsCopiedFalseWhenCopyFails(t *testing.T) {
	directory := t.TempDir()
	// Reading from a directory handle fails, so the copy errors before any
	// destination bytes matter.
	source, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	copied, err := WriteFile(filepath.Join(directory, "missing", "target"), source)
	if err == nil {
		t.Fatal("WriteFile() error = nil for unreadable source directory")
	}
	if copied {
		t.Fatal("WriteFile() copied = true for a failed copy")
	}
}
