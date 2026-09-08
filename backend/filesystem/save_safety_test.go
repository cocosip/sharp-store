package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	store "github.com/cocosip/sharp-store"
)

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) { return copy(p, "partial"), io.ErrUnexpectedEOF }

func TestBackend_SaveFailurePreservesDestination(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "overwrite"}[overwrite], func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "file")
			if overwrite {
				if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			request := store.SaveRequest{FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{RootKey: root}}, Key: "file", FileID: "file"}, Body: failingReader{}, Overwrite: overwrite}
			b := New()
			if _, err := b.Save(context.Background(), request); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("Save() = %v", err)
			}
			data, err := os.ReadFile(path)
			if overwrite {
				if err != nil || string(data) != "original" {
					t.Fatalf("original lost: %q, %v", data, err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed save published %q, %v", data, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if overwrite {
				want = 1
			}
			if len(entries) != want {
				t.Fatalf("temporary files remain: %v", entries)
			}
			request.Body = strings.NewReader("complete")
			if _, err := b.Save(context.Background(), request); err != nil {
				t.Fatalf("retry: %v", err)
			}
		})
	}
}

func TestBackend_RejectsParentSegments(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"tenant-a/../tenant-b/file", `tenant-a\..\tenant-b\file`} {
		request := store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{RootKey: root}}, Key: key}
		if _, err := New().Save(context.Background(), store.SaveRequest{FileRequest: request, Body: strings.NewReader("bad")}); err == nil {
			t.Errorf("Save accepted %q", key)
		}
	}
}

func TestBackend_SaveExclusiveConcurrent(t *testing.T) {
	root := t.TempDir()
	b := New()
	start := make(chan struct{})
	errs := make(chan error, 12)
	for range 12 {
		go func() {
			<-start
			_, err := b.Save(context.Background(), store.SaveRequest{FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{RootKey: root}}, Key: "file", FileID: "file"}, Body: strings.NewReader("complete")})
			errs <- err
		}()
	}
	close(start)
	successes := 0
	for range 12 {
		if err := <-errs; err == nil {
			successes++
		} else if !errors.Is(err, store.ErrFileExists) {
			t.Error(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful writes = %d", successes)
	}
}

type cancelingReader struct{ cancel context.CancelFunc }

func (r cancelingReader) Read(p []byte) (int, error) { r.cancel(); return copy(p, "partial"), io.EOF }

func TestBackend_SaveCanceledBeforePublish(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := New().Save(ctx, store.SaveRequest{FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{RootKey: root}}, Key: "file", FileID: "file"}, Body: cancelingReader{cancel: cancel}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Save(): %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("canceled save left files: %v, %v", entries, err)
	}
}

func TestBackend_SaveOverwritePreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions")
	}
	root := t.TempDir()
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := New().Save(context.Background(), store.SaveRequest{FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{RootKey: root}}, Key: "file", FileID: "file"}, Body: strings.NewReader("updated"), Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions changed to %v", info.Mode().Perm())
	}
}

func TestBackend_SaveFailureCleansReadOnlyTemporaryFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			_ = os.Chmod(filepath.Join(root, entry.Name()), 0o644)
		}
	})
	_, err := New().Save(context.Background(), store.SaveRequest{FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{RootKey: root}}, Key: "file", FileID: "file"}, Body: failingReader{}, Overwrite: true})
	if err == nil {
		t.Fatal("expected save failure")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "file" {
		t.Fatalf("temporary files remain: %v, %v", entries, err)
	}
}
