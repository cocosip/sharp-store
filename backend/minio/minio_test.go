package minio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestBackendValidateConfigRequiresMinIOConnectionSettings(t *testing.T) {
	valid := map[string]string{
		BucketKey:    "archive",
		EndpointKey:  "minio.example.test:9000",
		AccessKeyKey: "access",
		SecretKeyKey: "secret",
		RegionKey:    "cn-east-1",
		WithSSLKey:   "true",
	}
	for _, key := range []string{BucketKey, EndpointKey, AccessKeyKey, SecretKeyKey} {
		t.Run(key, func(t *testing.T) {
			values := cloneValues(valid)
			delete(values, key)
			if err := New().(store.BackendConfigValidator).ValidateConfig(context.Background(), values); err == nil {
				t.Fatalf("ValidateConfig() error = nil when %s is missing", key)
			}
		})
	}
}

func TestBackendSaveUsesMinIOSDKAndConditionalPut(t *testing.T) {
	var authorization, userAgent, ifNoneMatch, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		userAgent = r.UserAgent()
		ifNoneMatch = r.Header.Get("If-None-Match")
		path = r.URL.Path
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", RegionKey: "cn-east-1", WithSSLKey: "false",
			}},
			FileID: "file",
			Key:    "tenant/file.txt",
		},
		Body: bytes.NewBufferString("content"),
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !strings.Contains(userAgent, "minio-go") {
		t.Fatalf("User-Agent = %q, want MinIO SDK user agent", userAgent)
	}
	if !strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 ") || !strings.Contains(authorization, "/cn-east-1/s3/aws4_request") {
		t.Fatalf("Authorization = %q, want MinIO SigV4 authorization for cn-east-1", authorization)
	}
	if ifNoneMatch != "*" {
		t.Fatalf("If-None-Match = %q, want *", ifNoneMatch)
	}
	if path != "/archive/tenant/file.txt" {
		t.Fatalf("path = %q, want path-style MinIO object path", path)
	}
}

func TestBackendSaveFailsWhenUploadReaderPositionCannotBeRestored(t *testing.T) {
	requestSent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestSent = true
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", WithSSLKey: "false",
			}},
			FileID: "file", Key: "tenant/file.txt",
		},
		Body: &restoreFailingSeeker{data: []byte("content")},
	})
	if err == nil || !strings.Contains(err.Error(), "restore upload reader position") {
		t.Fatalf("Save() error = %v, want restore upload reader position error", err)
	}
	if requestSent {
		t.Fatal("Save() sent a request after failing to restore the upload reader")
	}
}

func TestBackendSaveDoesNotClassifyUnrelatedConflictAsExistingFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, "<Error><Code>OperationAborted</Code><Message>retry later</Message></Error>")
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", WithSSLKey: "false",
			}},
			FileID: "file", Key: "tenant/file.txt",
		},
		Body: bytes.NewBufferString("content"),
	})
	if err == nil || errors.Is(err, store.ErrFileExists) {
		t.Fatalf("Save() error = %v, want provider conflict distinct from ErrFileExists", err)
	}
}

type restoreFailingSeeker struct {
	data     []byte
	position int64
}

func (r *restoreFailingSeeker) Read(buffer []byte) (int, error) {
	if r.position >= int64(len(r.data)) {
		return 0, io.EOF
	}
	read := copy(buffer, r.data[r.position:])
	r.position += int64(read)
	return read, nil
}

func (r *restoreFailingSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		return r.position, nil
	case io.SeekEnd:
		r.position = int64(len(r.data)) + offset
		return r.position, nil
	case io.SeekStart:
		return r.position, errors.New("restore failed")
	default:
		return r.position, errors.New("unsupported seek")
	}
}

func TestBackendValidateConfigRejectsInvalidEndpointAndUseSSL(t *testing.T) {
	tests := []map[string]string{
		{BucketKey: "archive", EndpointKey: "https://minio.example.test:9000", AccessKeyKey: "access", SecretKeyKey: "secret"},
		{BucketKey: "archive", EndpointKey: "minio.example.test:9000/storage", AccessKeyKey: "access", SecretKeyKey: "secret"},
		{BucketKey: "archive", EndpointKey: "minio.example.test:9000", AccessKeyKey: "access", SecretKeyKey: "secret", WithSSLKey: "enabled"},
	}
	for _, values := range tests {
		if err := New().(store.BackendConfigValidator).ValidateConfig(context.Background(), values); err == nil {
			t.Fatalf("ValidateConfig(%v) error = nil", values)
		}
	}
}

func TestBackendConfigOptionsMatchMinIOSDK(t *testing.T) {
	want := []string{BucketKey, EndpointKey, AccessKeyKey, SecretKeyKey, RegionKey, WithSSLKey, CreateBucketIfNotExistsKey}
	options := New().(store.BackendDescriptor).ConfigOptions()
	if len(options) != len(want) {
		t.Fatalf("ConfigOptions() count = %d, want %d: %#v", len(options), len(want), options)
	}
	for index, name := range want {
		if options[index].Name != name {
			t.Fatalf("ConfigOptions()[%d].Name = %q, want %q", index, options[index].Name, name)
		}
	}
}

func TestConfigValuesUsesMinIOSDKSettings(t *testing.T) {
	values := NewConfig().
		WithBucket("archive").
		WithEndpoint("minio.example.test:9000").
		WithCredentials("access", "secret").
		WithRegion("cn-east-1").
		WithSSL(true).
		WithCreateBucketIfNotExists(true).
		Values()
	want := map[string]string{
		BucketKey: "archive", EndpointKey: "minio.example.test:9000",
		AccessKeyKey: "access", SecretKeyKey: "secret", RegionKey: "cn-east-1", WithSSLKey: "true",
		CreateBucketIfNotExistsKey: "true",
	}
	if !maps.Equal(values, want) {
		t.Fatalf("Values() = %#v, want %#v", values, want)
	}
}

func TestBackendSaveCreatesMissingBucketWhenConfigured(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodHead && r.URL.Path == "/archive/" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchBucket</Code></Error>")
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", WithSSLKey: "false",
				CreateBucketIfNotExistsKey: "true",
			}},
			FileID: "file",
			Key:    "tenant/file.txt",
		},
		Body: bytes.NewBufferString("content"),
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	want := []string{"HEAD /archive/", "PUT /archive/", "PUT /archive/tenant/file.txt"}
	if !slices.Equal(requests, want) {
		t.Fatalf("requests = %#v, want %#v", requests, want)
	}
}

func TestBackendSaveContinuesWhenBucketWasCreatedConcurrently(t *testing.T) {
	var objectUploaded bool
	headRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/archive/":
			headRequests++
			if headRequests == 1 {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, "<Error><Code>NoSuchBucket</Code></Error>")
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/archive/":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, "<Error><Code>BucketAlreadyExists</Code></Error>")
		case r.Method == http.MethodPut && r.URL.Path == "/archive/tenant/file.txt":
			objectUploaded = true
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", WithSSLKey: "false",
				CreateBucketIfNotExistsKey: "true",
			}},
			FileID: "file", Key: "tenant/file.txt",
		},
		Body: bytes.NewBufferString("content"),
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !objectUploaded {
		t.Fatal("Save() did not upload after concurrent bucket creation")
	}
	if headRequests != 2 {
		t.Fatalf("BucketExists requests = %d, want 2", headRequests)
	}
}

func cloneValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
