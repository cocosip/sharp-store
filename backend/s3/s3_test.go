package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	store "github.com/cocosip/sharp-store"
)

func TestBackendSaveAndGetAgainstS3CompatibleEndpoint(t *testing.T) {
	t.Parallel()

	server := newS3CompatibleServer(t)

	backend := New()
	request := saveRequest(server.URL, "instance", "tenant-a/study/instance.dcm", "pixel-data", false)

	fileID, err := backend.Save(context.Background(), request)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if fileID != "instance" {
		t.Fatalf("Save() file ID = %q, want %q", fileID, "instance")
	}

	reader, err := backend.GetOrNil(context.Background(), request.FileRequest)
	if err != nil {
		t.Fatalf("GetOrNil() error = %v", err)
	}
	if reader == nil {
		t.Fatal("GetOrNil() reader = nil")
	}
	defer func() { _ = reader.Close() }()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(content) != "pixel-data" {
		t.Fatalf("GetOrNil() content = %q, want %q", content, "pixel-data")
	}
}

func TestBackendGetOrNilReturnsNilForMissingObject(t *testing.T) {
	t.Parallel()

	backend := New()
	reader, err := backend.GetOrNil(context.Background(), saveRequest(newS3CompatibleServer(t).URL, "missing", "tenant-a/missing", "", false).FileRequest)
	if err != nil {
		t.Fatalf("GetOrNil() error = %v", err)
	}
	if reader != nil {
		defer func() { _ = reader.Close() }()
		t.Fatal("GetOrNil() reader is not nil for a missing object")
	}
}

func TestBackendRejectsExistingObjectWhenOverwriteDisabled(t *testing.T) {
	t.Parallel()

	server := newS3CompatibleServer(t)
	backend := New()
	first := saveRequest(server.URL, "instance", "tenant-a/instance", "first", false)
	if _, err := backend.Save(context.Background(), first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	_, err := backend.Save(context.Background(), saveRequest(server.URL, "instance", "tenant-a/instance", "second", false))
	if !errors.Is(err, store.ErrFileExists) {
		t.Fatalf("second Save() error = %v, want ErrFileExists", err)
	}
}

func TestBackendExistsAndDelete(t *testing.T) {
	t.Parallel()

	server := newS3CompatibleServer(t)
	backend := New()
	request := saveRequest(server.URL, "instance", "tenant-a/instance", "pixel-data", false)
	if _, err := backend.Save(context.Background(), request); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	exists, err := backend.Exists(context.Background(), request.FileRequest)
	if err != nil || !exists {
		t.Fatalf("Exists() = (%t, %v), want (true, nil)", exists, err)
	}
	deleted, err := backend.Delete(context.Background(), request.FileRequest)
	if err != nil || !deleted {
		t.Fatalf("Delete() = (%t, %v), want (true, nil)", deleted, err)
	}
	exists, err = backend.Exists(context.Background(), request.FileRequest)
	if err != nil || exists {
		t.Fatalf("Exists() after Delete = (%t, %v), want (false, nil)", exists, err)
	}
	deleted, err = backend.Delete(context.Background(), request.FileRequest)
	if err != nil || deleted {
		t.Fatalf("Delete() for missing object = (%t, %v), want (false, nil)", deleted, err)
	}
}

func TestBackendAccessURLPresignsExistingObject(t *testing.T) {
	t.Parallel()

	server := newS3CompatibleServer(t)
	backend := New()
	request := saveRequest(server.URL, "instance", "tenant-a/instance", "pixel-data", false)
	if _, err := backend.Save(context.Background(), request); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	expiresAt := time.Now().Add(5 * time.Minute)
	accessURL, err := backend.AccessURL(context.Background(), store.AccessURLRequest{
		FileRequest: request.FileRequest,
		ExpiresAt:   &expiresAt,
		CheckExists: true,
	})
	if err != nil {
		t.Fatalf("AccessURL() error = %v", err)
	}
	parsed, err := url.Parse(accessURL)
	if err != nil {
		t.Fatalf("AccessURL() returned invalid URL %q: %v", accessURL, err)
	}
	if parsed.Scheme != "http" || parsed.Host != server.Listener.Addr().String() {
		t.Fatalf("AccessURL() = %q, want URL for %q", accessURL, server.URL)
	}
	if parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("AccessURL() = %q, want X-Amz-Signature", accessURL)
	}
}

func saveRequest(endpoint, fileID, key, content string, overwrite bool) store.SaveRequest {
	return store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey:          "archive",
				RegionKey:          "us-east-1",
				EndpointKey:        endpoint,
				AccessKeyIDKey:     "access",
				SecretAccessKeyKey: "secret",
				PathStyleKey:       "true",
			}},
			FileID: fileID,
			Key:    key,
		},
		Body:      bytes.NewBufferString(content),
		Overwrite: overwrite,
	}
}

func newS3CompatibleServer(t *testing.T) *httptest.Server {
	t.Helper()
	objects := make(map[string][]byte)
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("If-None-Match") == "*" {
				if _, exists := objects[r.URL.Path]; exists {
					w.WriteHeader(http.StatusPreconditionFailed)
					return
				}
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			objects[r.URL.Path] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(body)
		case http.MethodHead:
			if _, ok := objects[r.URL.Path]; !ok {
				w.WriteHeader(http.StatusNotFound)
			}
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
