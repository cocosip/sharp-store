package aws

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	store "github.com/cocosip/sharp-store"
)

func TestConfigPreservesAWSConnectionSettings(t *testing.T) {
	values := NewConfig().
		WithBucket("archive").
		WithRegion("ap-southeast-1").
		WithCredentials("access", "secret").
		WithSessionToken("token").
		WithCreateContainerIfNotExists(true).
		Values()
	if got, want := values[SessionTokenKey], "token"; got != want {
		t.Fatalf("session token = %q, want %q", got, want)
	}
	if got, want := values[CreateContainerIfNotExistsKey], "true"; got != want {
		t.Fatalf("create container = %q, want %q", got, want)
	}
}

func TestBackendSaveCreatesMissingBucketWhenConfigured(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodHead && r.URL.Path == "/archive" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchBucket</Code></Error>")
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	backend := &Backend{baseEndpoint: server.URL, forcePathStyle: true}
	_, err := backend.Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", RegionKey: "us-east-1",
				AccessKeyIDKey: "access", SecretAccessKeyKey: "secret",
				CreateContainerIfNotExistsKey: "true",
			}},
			FileID: "file", Key: "tenant/file.txt",
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

	backend := &Backend{baseEndpoint: server.URL, forcePathStyle: true}
	_, err := backend.Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{
			Config: store.ContainerConfig{Values: map[string]string{
				BucketKey: "archive", RegionKey: "us-east-1",
				AccessKeyIDKey: "access", SecretAccessKeyKey: "secret",
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

func TestBackendValidateConfigAllowsDefaultCredentialChain(t *testing.T) {
	err := New().(store.BackendConfigValidator).ValidateConfig(context.Background(), map[string]string{
		BucketKey: "archive",
	})
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v, want default credential chain", err)
	}
}

func TestBackendConfigOptionsMatchAWSSDK(t *testing.T) {
	want := []string{BucketKey, RegionKey, AccessKeyIDKey, SecretAccessKeyKey, SessionTokenKey, CreateContainerIfNotExistsKey}
	options := New().(store.BackendDescriptor).ConfigOptions()
	got := make([]string, len(options))
	for index := range options {
		got[index] = options[index].Name
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ConfigOptions() = %#v, want %#v", got, want)
	}
}
