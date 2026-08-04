package s3compat_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/aws"
	"github.com/cocosip/sharp-store/backend/ks3"
	"github.com/cocosip/sharp-store/backend/minio"
)

func TestS3CompatibleProvidersSaveAndGet(t *testing.T) {
	t.Parallel()

	server := newObjectServer(t)
	tests := []struct {
		name    string
		backend store.Backend
		values  map[string]string
	}{
		{
			name:    "aws",
			backend: aws.New(),
			values: map[string]string{
				aws.BucketKey:          "archive",
				aws.RegionKey:          "us-east-1",
				aws.EndpointKey:        server.URL,
				aws.AccessKeyIDKey:     "access",
				aws.SecretAccessKeyKey: "secret",
				aws.PathStyleKey:       "true",
			},
		},
		{
			name:    "minio",
			backend: minio.New(),
			values: map[string]string{
				minio.BucketKey:    "archive",
				minio.EndpointKey:  server.URL,
				minio.AccessKeyKey: "access",
				minio.SecretKeyKey: "secret",
				minio.UseSSLKey:    "false",
			},
		},
		{
			name:    "ks3",
			backend: ks3.New(),
			values: map[string]string{
				ks3.BucketKey:    "archive",
				ks3.EndpointKey:  server.URL,
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
			reader, err := test.backend.GetOrNil(context.Background(), request.FileRequest)
			if err != nil {
				t.Fatalf("GetOrNil() error = %v", err)
			}
			if reader == nil {
				t.Fatal("GetOrNil() reader = nil")
			}
			defer func() { _ = reader.Close() }()
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if got, want := string(body), "content-"+test.name; got != want {
				t.Fatalf("GetOrNil() = %q, want %q", got, want)
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
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
