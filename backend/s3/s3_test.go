package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
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

func TestBackendValidateConfigRequiresStaticCredentials(t *testing.T) {
	err := New().(store.BackendConfigValidator).ValidateConfig(context.Background(), map[string]string{
		BucketKey: "archive",
	})
	if err == nil {
		t.Fatal("ValidateConfig() error = nil without access_key_id and secret_access_key")
	}
}

func TestBackendConfigOptionsMatchStandardS3Provider(t *testing.T) {
	want := []string{
		BucketKey, BaseEndpointKey, AccessKeyIDKey, SecretAccessKeyKey,
		ForcePathStyleKey, UseChunkEncodingKey, ProtocolKey, CreateBucketIfNotExistsKey,
	}
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

func TestConfigValuesUsesStandardS3Settings(t *testing.T) {
	values := NewConfig().
		WithBucket("archive").
		WithBaseEndpoint("https://s3.example.test").
		WithCredentials("access", "secret").
		WithForcePathStyle(true).
		WithUseChunkEncoding(true).
		WithProtocol("http").
		WithCreateBucketIfNotExists(true).
		Values()
	want := map[string]string{
		BucketKey: "archive", BaseEndpointKey: "https://s3.example.test",
		AccessKeyIDKey: "access", SecretAccessKeyKey: "secret",
		ForcePathStyleKey: "true", UseChunkEncodingKey: "true", ProtocolKey: "http",
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
		if r.Method == http.MethodHead && r.URL.Path == "/archive" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	request := saveRequest(server.URL, "instance", "tenant-a/instance", "content", false)
	request.Config.Values[CreateBucketIfNotExistsKey] = "true"
	if _, err := New().Save(context.Background(), request); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	want := []string{"HEAD /archive", "PUT /archive", "PUT /archive/tenant-a/instance"}
	if !slices.Equal(requests, want) {
		t.Fatalf("requests = %#v, want %#v", requests, want)
	}
}

func TestBackendSaveContinuesWhenBucketWasCreatedConcurrently(t *testing.T) {
	var objectUploaded bool
	headRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/archive":
			headRequests++
			if headRequests == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/archive":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, "<Error><Code>BucketAlreadyExists</Code></Error>")
		case r.Method == http.MethodPut && r.URL.Path == "/archive/tenant-a/instance":
			objectUploaded = true
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	request := saveRequest(server.URL, "instance", "tenant-a/instance", "content", false)
	request.Config.Values[CreateBucketIfNotExistsKey] = "true"
	if _, err := New().Save(context.Background(), request); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !objectUploaded {
		t.Fatal("Save() did not upload after concurrent bucket creation")
	}
	if headRequests != 2 {
		t.Fatalf("HeadBucket requests = %d, want 2", headRequests)
	}
}

func TestBackendAccessURLUsesConfiguredProtocol(t *testing.T) {
	server := newS3CompatibleServer(t)
	request := saveRequest(server.URL, "instance", "tenant-a/instance", "content", false)
	backend := New()
	if _, err := backend.Save(context.Background(), request); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	request.Config.Values[ProtocolKey] = "https"
	accessURL, err := backend.AccessURL(context.Background(), store.AccessURLRequest{FileRequest: request.FileRequest})
	if err != nil {
		t.Fatalf("AccessURL() error = %v", err)
	}
	parsed, err := url.Parse(accessURL)
	if err != nil || parsed.Scheme != "https" {
		t.Fatalf("AccessURL() = %q, error = %v, want https scheme", accessURL, err)
	}
}

func TestBackendUseChunkEncodingControlsUnknownLengthUpload(t *testing.T) {
	tests := []struct {
		name        string
		useChunk    bool
		wantLength  int64
		wantChunked bool
	}{
		{name: "fixed length", useChunk: false, wantLength: 7},
		{name: "chunked", useChunk: true, wantLength: -1, wantChunked: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var contentLength int64
			var chunked bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contentLength = r.ContentLength
				chunked = slices.Contains(r.TransferEncoding, "chunked")
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			request := saveRequest(server.URL, "instance", "tenant-a/instance", "", false)
			request.Config.Values[UseChunkEncodingKey] = strconv.FormatBool(test.useChunk)
			request.Body = readerOnly{Reader: bytes.NewBufferString("content")}
			if _, err := New().Save(context.Background(), request); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			if contentLength != test.wantLength || chunked != test.wantChunked {
				t.Fatalf("request = (ContentLength %d, chunked %t), want (%d, %t)", contentLength, chunked, test.wantLength, test.wantChunked)
			}
		})
	}
}

type readerOnly struct{ io.Reader }

func saveRequest(endpoint, fileID, key, content string, overwrite bool) store.SaveRequest {
	return store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey:          "archive",
				BaseEndpointKey:    endpoint,
				AccessKeyIDKey:     "access",
				SecretAccessKeyKey: "secret",
				ForcePathStyleKey:  "true",
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
