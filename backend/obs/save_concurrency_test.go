package obs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	store "github.com/cocosip/sharp-store"
)

func TestBackend_SavePreservesOtherConflicts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("x-ms-error-code", "BucketAlreadyExists")
		w.Header().Set("x-obs-error-code", "BucketAlreadyExists")
		w.WriteHeader(409)
		_, _ = io.WriteString(w, "<Error><Code>BucketAlreadyExists</Code><Message>conflict</Message></Error>")
	}))
	defer server.Close()
	_, err := New().Save(context.Background(), store.SaveRequest{
		FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{EndpointKey: server.URL, BucketKey: "archive", AccessKeyIDKey: "access", AccessKeySecretKey: "secret"}}, Key: "file", FileID: "file"},
		Body:        strings.NewReader("data"),
	})
	if err == nil || errors.Is(err, store.ErrFileExists) {
		t.Fatalf("unrelated conflict mapped incorrectly: %v", err)
	}
}

func TestBackend_SaveNoOverwriteIsAtomic(t *testing.T) {
	var arrivals atomic.Int32
	ready := make(chan struct{})
	var mu sync.Mutex
	var object []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if arrivals.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			w.WriteHeader(500)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("x-obs-error-code", "NoSuchKey")
			w.WriteHeader(404)
			return
		}
		if r.Method != http.MethodPut {
			w.WriteHeader(405)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if object != nil && r.Header.Get("x-obs-forbid-overwrite") == "true" {
			w.Header().Set("Content-Type", "application/xml")
			w.Header().Set("x-ms-error-code", "ObjectAlreadyExists")
			w.Header().Set("x-obs-error-code", "ObjectAlreadyExists")
			w.WriteHeader(409)
			_, _ = io.WriteString(w, "<Error><Code>ObjectAlreadyExists</Code><Message>exists</Message></Error>")
			return
		}
		object = data
		w.Header().Set("ETag", "\"local\"")
		w.WriteHeader(200)
	}))
	defer server.Close()
	backend := New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errs := make(chan error, 2)
	for _, body := range []string{"first", "second"} {
		go func() {
			_, err := backend.Save(ctx, store.SaveRequest{
				FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{EndpointKey: server.URL, BucketKey: "archive", AccessKeyIDKey: "access", AccessKeySecretKey: "secret"}}, Key: "same", FileID: "same"},
				Body:        strings.NewReader(body),
			})
			errs <- err
		}()
	}
	successes, conflicts := 0, 0
	for range 2 {
		err := <-errs
		if err == nil {
			successes++
		} else if errors.Is(err, store.ErrFileExists) {
			conflicts++
		} else {
			t.Errorf("Save() error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want 1 each", successes, conflicts)
	}
	request := store.SaveRequest{
		FileRequest: store.FileRequest{Config: store.ContainerConfig{Values: map[string]string{EndpointKey: server.URL, BucketKey: "archive", AccessKeyIDKey: "access", AccessKeySecretKey: "secret"}}, Key: "same", FileID: "same"},
		Body:        strings.NewReader("replacement"), Overwrite: true,
	}
	if _, err := backend.Save(ctx, request); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if string(object) != "replacement" {
		t.Fatalf("overwrite saved %q", object)
	}
}
