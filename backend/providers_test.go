package backend_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/ks3"
	"github.com/cocosip/sharp-store/backend/minio"
)

func TestS3CompatibleProvidersHonorObjectContract(t *testing.T) {
	t.Parallel()

	server := newObjectServer(t)
	tests := []struct {
		name    string
		backend store.Backend
		values  map[string]string
	}{
		{
			name:    "minio",
			backend: minio.New(),
			values: map[string]string{
				minio.BucketKey:    "archive",
				minio.EndpointKey:  server.Listener.Addr().String(),
				minio.AccessKeyKey: "access",
				minio.SecretKeyKey: "secret",
				minio.WithSSLKey:   "false",
			},
		},
		{
			name:    "ks3",
			backend: ks3.New(),
			values: map[string]string{
				ks3.BucketKey:    "archive",
				ks3.EndpointKey:  server.Listener.Addr().String(),
				ks3.AccessKeyKey: "access",
				ks3.SecretKeyKey: "secret",
				ks3.ProtocolKey:  "http",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := store.SaveRequest{
				FileRequest: store.FileRequest{
					Config: store.ContainerConfig{Values: test.values},
					FileID: "file",
					Key:    test.name + "/file.txt",
				},
				Body: bytes.NewBufferString("content-" + test.name),
			}
			if _, err := test.backend.Save(context.Background(), request); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			exists, err := test.backend.Exists(context.Background(), request.FileRequest)
			if err != nil || !exists {
				t.Fatalf("Exists() = (%t, %v), want (true, nil)", exists, err)
			}
			reader, err := test.backend.GetOrNil(context.Background(), request.FileRequest)
			if err != nil {
				t.Fatalf("GetOrNil() error = %v", err)
			}
			if reader == nil {
				t.Fatal("GetOrNil() reader = nil")
			}
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if got, want := string(body), "content-"+test.name; got != want {
				t.Fatalf("GetOrNil() = %q, want %q", got, want)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			_, err = test.backend.Save(context.Background(), request)
			if !errors.Is(err, store.ErrFileExists) {
				t.Fatalf("second Save() error = %v, want ErrFileExists", err)
			}
			expiresAt := time.Now().Add(time.Minute)
			accessURL, err := test.backend.AccessURL(context.Background(), store.AccessURLRequest{
				FileRequest: request.FileRequest,
				ExpiresAt:   &expiresAt,
				CheckExists: true,
			})
			if err != nil {
				t.Fatalf("AccessURL() error = %v", err)
			}
			parsed, err := url.Parse(accessURL)
			signature := parsed.Query().Get("X-Amz-Signature")
			if test.name == "ks3" {
				signature = parsed.Query().Get("Signature")
			}
			if err != nil || signature == "" {
				t.Fatalf("AccessURL() = %q, error = %v, want signed URL", accessURL, err)
			}
			destination := filepath.Join(t.TempDir(), "downloaded.txt")
			downloaded, err := test.backend.Download(context.Background(), store.DownloadRequest{
				FileRequest: request.FileRequest,
				Destination: destination,
			})
			if err != nil || !downloaded {
				t.Fatalf("Download() = (%t, %v), want (true, nil)", downloaded, err)
			}
			downloadedContent, err := os.ReadFile(destination)
			if err != nil || string(downloadedContent) != "content-"+test.name {
				t.Fatalf("downloaded content = %q, error = %v", downloadedContent, err)
			}

			deleted, err := test.backend.Delete(context.Background(), request.FileRequest)
			if err != nil || !deleted {
				t.Fatalf("Delete() = (%t, %v), want (true, nil)", deleted, err)
			}
			exists, err = test.backend.Exists(context.Background(), request.FileRequest)
			if err != nil || exists {
				t.Fatalf("Exists() after Delete = (%t, %v), want (false, nil)", exists, err)
			}
			reader, err = test.backend.GetOrNil(context.Background(), request.FileRequest)
			if err != nil || reader != nil {
				if reader != nil {
					defer func() { _ = reader.Close() }()
				}
				t.Fatalf("GetOrNil() after Delete = (%v, %v), want (nil, nil)", reader, err)
			}
			_, err = test.backend.AccessURL(context.Background(), store.AccessURLRequest{
				FileRequest: request.FileRequest,
				CheckExists: true,
			})
			if !errors.Is(err, store.ErrFileNotFound) {
				t.Fatalf("AccessURL() for missing object error = %v, want ErrFileNotFound", err)
			}
		})
	}
}

func newObjectServer(t *testing.T) *httptest.Server {
	t.Helper()
	objects := make(map[string][]byte)
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("If-None-Match") == "*" || r.Header.Get("x-amz-forbid-overwrite") == "true" {
				if _, exists := objects[r.URL.Path]; exists {
					code := "PreconditionFailed"
					if r.Header.Get("x-amz-forbid-overwrite") == "true" {
						code = "ObjectAlreadyExists"
					}
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusPreconditionFailed)
					_, _ = io.WriteString(w, "<Error><Code>"+code+"</Code><Message>object exists</Message></Error>")
					return
				}
			}
			body, err := readRequestBody(r)
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
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.Header().Set("ETag", `"test-etag"`)
			_, _ = w.Write(body)
		case http.MethodHead:
			body, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.Header().Set("ETag", `"test-etag"`)
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

func readRequestBody(request *http.Request) ([]byte, error) {
	if !strings.Contains(request.Header.Get("Content-Encoding"), "aws-chunked") {
		return io.ReadAll(request.Body)
	}

	reader := bufio.NewReader(request.Body)
	var decoded bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeText := strings.SplitN(strings.TrimSpace(line), ";", 2)[0]
		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil {
			return nil, err
		}
		if size == 0 {
			return decoded.Bytes(), nil
		}
		if _, err := io.CopyN(&decoded, reader, size); err != nil {
			return nil, err
		}
		if _, err := reader.Discard(2); err != nil {
			return nil, err
		}
	}
}
