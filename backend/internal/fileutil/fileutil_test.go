package fileutil

import (
	"errors"
	"io"
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

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) { return copy(p, "partial"), io.ErrUnexpectedEOF }

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

func TestWriteFileFailurePreservesExistingDestination(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "object.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	copied, err := WriteFile(destination, failingReader{})
	if !errors.Is(err, io.ErrUnexpectedEOF) || copied {
		t.Fatalf("WriteFile() = (%t, %v), want (false, io.ErrUnexpectedEOF)", copied, err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "original" {
		t.Fatalf("destination = %q, error = %v, want original preserved", data, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "object.txt" {
		t.Fatalf("directory entries = %v, %v, want no temporary leftovers", entries, err)
	}
}
