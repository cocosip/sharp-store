package ks3

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
	"time"

	store "github.com/cocosip/sharp-store"
)

func TestBackendValidateConfigRequiresKS3ConnectionSettings(t *testing.T) {
	valid := map[string]string{
		BucketKey:    "archive",
		EndpointKey:  "ks3.example.test",
		AccessKeyKey: "access",
		SecretKeyKey: "secret",
		ProtocolKey:  "https",
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

func TestBackendConfigOptionsMatchKS3SDK(t *testing.T) {
	want := []string{
		BucketKey, EndpointKey, AccessKeyKey, SecretKeyKey, ProtocolKey,
		UserAgentKey, MaxConnectionsKey, TimeoutKey, CreateContainerIfNotExistsKey,
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

func TestConfigValuesUsesKS3SDKSettings(t *testing.T) {
	values := NewConfig().
		WithBucket("archive").
		WithEndpoint("ks3.example.test").
		WithCredentials("access", "secret").
		WithProtocol("https").
		WithUserAgent("sharp-store-tests").
		WithMaxConnections(20).
		WithTimeout(120000).
		WithCreateContainerIfNotExists(true).
		Values()
	want := map[string]string{
		BucketKey: "archive", EndpointKey: "ks3.example.test",
		AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "https",
		UserAgentKey: "sharp-store-tests", MaxConnectionsKey: "20", TimeoutKey: "120000",
		CreateContainerIfNotExistsKey: "true",
	}
	if !maps.Equal(values, want) {
		t.Fatalf("Values() = %#v, want %#v", values, want)
	}
}

func TestBackendSaveUsesKS3V2SignatureAndForbidOverwrite(t *testing.T) {
	var authorization, forbidOverwrite, ifNoneMatch, path, body, userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		forbidOverwrite = r.Header.Get("x-amz-forbid-overwrite")
		ifNoneMatch = r.Header.Get("If-None-Match")
		path = r.URL.Path
		userAgent = r.UserAgent()
		content, _ := io.ReadAll(r.Body)
		body = string(content)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "http",
				UserAgentKey: "sharp-store-tests",
			}},
			FileID: "file",
			Key:    "tenant/file.txt",
		},
		Body: bytes.NewBufferString("content"),
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !strings.HasPrefix(authorization, "AWS access:") {
		t.Fatalf("Authorization = %q, want KS3 V2 authorization", authorization)
	}
	if forbidOverwrite != "true" {
		t.Fatalf("x-amz-forbid-overwrite = %q, want true", forbidOverwrite)
	}
	if ifNoneMatch != "" {
		t.Fatalf("If-None-Match = %q, want KS3 native overwrite header only", ifNoneMatch)
	}
	if path != "/archive/tenant/file.txt" {
		t.Fatalf("path = %q, want path-style KS3 object path", path)
	}
	if body != "content" {
		t.Fatalf("body = %q, want uploaded content", body)
	}
	if userAgent != "sharp-store-tests" {
		t.Fatalf("User-Agent = %q, want sharp-store-tests", userAgent)
	}
}

func TestBackendClientAppliesConnectionSettings(t *testing.T) {
	client, cfg, err := (&Backend{}).client(context.Background(), map[string]string{
		BucketKey: "archive", EndpointKey: "ks3.example.test",
		AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "https",
		MaxConnectionsKey: "17", TimeoutKey: "120000",
	})
	if err != nil {
		t.Fatalf("client() error = %v", err)
	}
	transport, ok := client.Config.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("HTTP transport type = %T, want *http.Transport", client.Config.HTTPClient.Transport)
	}
	if transport.MaxConnsPerHost != 17 || transport.MaxIdleConnsPerHost != 17 {
		t.Fatalf("connection limits = (%d, %d), want (17, 17)", transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost)
	}
	if client.Config.HTTPClient.Timeout != 120*time.Second || cfg.timeout != 120000 {
		t.Fatalf("timeout = %s, config = %d, want 2m and 120000", client.Config.HTTPClient.Timeout, cfg.timeout)
	}
}

func TestBackendSaveUploadsReaderWithoutKnownLength(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, _ := io.ReadAll(r.Body)
		body = string(content)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "http",
			}},
			FileID: "file",
			Key:    "tenant/file.txt",
		},
		Body: struct{ io.Reader }{Reader: strings.NewReader("streamed-content")},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if body != "streamed-content" {
		t.Fatalf("body = %q, want uploaded content", body)
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
				AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "http",
			}},
			FileID: "file", Key: "tenant/file.txt",
		},
		Body: bytes.NewBufferString("content"),
	})
	if err == nil || errors.Is(err, store.ErrFileExists) {
		t.Fatalf("Save() error = %v, want provider conflict distinct from ErrFileExists", err)
	}
}

func TestBackendSaveCreatesMissingContainerWhenConfigured(t *testing.T) {
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

	endpoint := strings.TrimPrefix(server.URL, "http://")
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", EndpointKey: endpoint,
				AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "http",
				CreateContainerIfNotExistsKey: "true",
			}},
			FileID: "file",
			Key:    "tenant/file.txt",
		},
		Body: bytes.NewBufferString("content"),
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	want := []string{"HEAD /archive", "PUT /archive", "PUT /archive/tenant/file.txt"}
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
				AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "http",
				CreateContainerIfNotExistsKey: "true",
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
		t.Fatalf("HeadBucket requests = %d, want 2", headRequests)
	}
}

func TestBackendValidateConfigRejectsInvalidEndpointAndProtocol(t *testing.T) {
	tests := []map[string]string{
		{BucketKey: "archive", EndpointKey: "https://ks3.example.test", AccessKeyKey: "access", SecretKeyKey: "secret"},
		{BucketKey: "archive", EndpointKey: "ks3.example.test/storage", AccessKeyKey: "access", SecretKeyKey: "secret"},
		{BucketKey: "archive", EndpointKey: "ks3.example.test", AccessKeyKey: "access", SecretKeyKey: "secret", ProtocolKey: "ftp"},
	}
	for _, values := range tests {
		if err := New().(store.BackendConfigValidator).ValidateConfig(context.Background(), values); err == nil {
			t.Fatalf("ValidateConfig(%v) error = nil", values)
		}
	}
}

func cloneValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
