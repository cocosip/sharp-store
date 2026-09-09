package minio

import (
	"testing"

	"github.com/cocosip/sharp-store/backend/s3"
)

func TestConfigTranslatorAddsHTTPSForSchemeLessEndpoint(t *testing.T) {
	values, err := (configTranslator{}).Translate(map[string]string{
		BucketKey:    "archive",
		EndpointKey:  "minio.example.test:9000",
		AccessKeyKey: "access",
		SecretKeyKey: "secret",
		UseSSLKey:    "true",
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got, want := values[s3.EndpointKey], "https://minio.example.test:9000"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
	if got, want := values[s3.PathStyleKey], "true"; got != want {
		t.Fatalf("path style = %q, want %q", got, want)
	}
}

func TestConfigTranslatorRejectsInvalidUseSSL(t *testing.T) {
	_, err := (configTranslator{}).Translate(map[string]string{
		EndpointKey: "minio.example.test:9000",
		UseSSLKey:   "enabled",
	})
	if err == nil {
		t.Fatal("Translate() error = nil, want invalid use_ssl error")
	}
}

func TestConfigTranslatorRejectsEndpointOutsideHostPortFormat(t *testing.T) {
	tests := []string{
		"https://minio.example.test:9000",
		"minio.example.test:9000/storage",
	}
	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			_, err := (configTranslator{}).Translate(map[string]string{
				EndpointKey: endpoint,
			})
			if err == nil {
				t.Fatalf("Translate() error = nil for endpoint %q, want host[:port] validation error", endpoint)
			}
		})
	}
}
