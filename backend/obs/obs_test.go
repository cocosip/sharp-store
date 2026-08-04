package obs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestBackendValidatesConfig(t *testing.T) {
	backends := store.NewBackendRegistry(New())
	if err := backends.ValidateConfig(context.Background(), Name, nil); err == nil {
		t.Fatal("ValidateConfig() error = nil")
	}
	if err := backends.ValidateConfig(context.Background(), Name, map[string]string{EndpointKey: "https://obs.example.test", BucketKey: "archive", AccessKeyIDKey: "access", AccessKeySecretKey: "secret"}); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestBackendHonorsObjectContractAgainstLocalEndpoint(t *testing.T) {
	objects := make(map[string][]byte)
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const objectPath = "/archive/tenant-a/instance.dcm"
		if r.URL.Path != objectPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			objects[r.URL.Path] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			if _, ok := objects[r.URL.Path]; !ok {
				writeOBSError(w)
			}
		case http.MethodGet:
			body, ok := objects[r.URL.Path]
			if !ok {
				writeOBSError(w)
				return
			}
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write(body)
		case http.MethodDelete:
			if _, ok := objects[r.URL.Path]; !ok {
				writeOBSError(w)
				return
			}
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	backend := New()
	request := store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				EndpointKey:        server.URL,
				BucketKey:          "archive",
				AccessKeyIDKey:     "access",
				AccessKeySecretKey: "secret",
			}},
			FileID: "instance",
			Key:    "tenant-a/instance.dcm",
		},
		Body: bytes.NewBufferString("pixel-data"),
	}
	if _, err := backend.Save(context.Background(), request); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	exists, err := backend.Exists(context.Background(), request.FileRequest)
	if err != nil || !exists {
		t.Fatalf("Exists() = (%t, %v), want (true, nil)", exists, err)
	}
	reader, err := backend.GetOrNil(context.Background(), request.FileRequest)
	if err != nil || reader == nil {
		t.Fatalf("GetOrNil() = (%v, %v), want reader", reader, err)
	}
	content, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil || string(content) != "pixel-data" {
		t.Fatalf("GetOrNil() content = %q, error = %v", content, err)
	}
	if _, err := backend.Save(context.Background(), request); !errors.Is(err, store.ErrFileExists) {
		t.Fatalf("second Save() error = %v, want ErrFileExists", err)
	}
	destination := filepath.Join(t.TempDir(), "downloaded.dcm")
	downloaded, err := backend.Download(context.Background(), store.DownloadRequest{
		FileRequest: request.FileRequest,
		Destination: destination,
	})
	if err != nil || !downloaded {
		t.Fatalf("Download() = (%t, %v), want (true, nil)", downloaded, err)
	}
	content, err = os.ReadFile(destination)
	if err != nil || string(content) != "pixel-data" {
		t.Fatalf("downloaded content = %q, error = %v", content, err)
	}
	accessURL, err := backend.AccessURL(context.Background(), store.AccessURLRequest{FileRequest: request.FileRequest})
	if err != nil {
		t.Fatalf("AccessURL() error = %v", err)
	}
	parsed, err := url.Parse(accessURL)
	if err != nil || parsed.Query().Get("AWSAccessKeyId") != "access" || parsed.Query().Get("Signature") == "" {
		t.Fatalf("AccessURL() = %q, error = %v, want signed OBS URL", accessURL, err)
	}
	deleted, err := backend.Delete(context.Background(), request.FileRequest)
	if err != nil || !deleted {
		t.Fatalf("Delete() = (%t, %v), want (true, nil)", deleted, err)
	}
	reader, err = backend.GetOrNil(context.Background(), request.FileRequest)
	if err != nil || reader != nil {
		if reader != nil {
			defer func() { _ = reader.Close() }()
		}
		t.Fatalf("GetOrNil() after Delete = (%v, %v), want (nil, nil)", reader, err)
	}
	if _, err := backend.AccessURL(context.Background(), store.AccessURLRequest{FileRequest: request.FileRequest, CheckExists: true}); !errors.Is(err, store.ErrFileNotFound) {
		t.Fatalf("AccessURL() for missing object error = %v, want ErrFileNotFound", err)
	}
}

func writeOBSError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-obs-error-code", "NoSuchKey")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<Error><Code>NoSuchKey</Code><Message>not found</Message><RequestId>local</RequestId><HostId>local</HostId></Error>"))
}
