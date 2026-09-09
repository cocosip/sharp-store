package ks3

import (
	"testing"

	"github.com/cocosip/sharp-store/backend/s3"
)

func TestConfigTranslatorAddsConfiguredProtocolForSchemeLessEndpoint(t *testing.T) {
	values, err := (configTranslator{}).Translate(map[string]string{
		BucketKey:    "archive",
		EndpointKey:  "ks3.example.test",
		AccessKeyKey: "access",
		SecretKeyKey: "secret",
		ProtocolKey:  "http",
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got, want := values[s3.EndpointKey], "http://ks3.example.test"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
	if got, want := values[s3.PathStyleKey], "true"; got != want {
		t.Fatalf("path style = %q, want %q", got, want)
	}
}

func TestConfigTranslatorRejectsInvalidProtocol(t *testing.T) {
	_, err := (configTranslator{}).Translate(map[string]string{
		EndpointKey: "ks3.example.test",
		ProtocolKey: "ftp",
	})
	if err == nil {
		t.Fatal("Translate() error = nil, want invalid protocol error")
	}
}

func TestConfigTranslatorDefaultsSchemeLessEndpointToHTTP(t *testing.T) {
	values, err := (configTranslator{}).Translate(map[string]string{
		EndpointKey: "ks3.example.test",
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got, want := values[s3.EndpointKey], "http://ks3.example.test"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

func TestConfigTranslatorRejectsEndpointOutsideHostPortFormat(t *testing.T) {
	tests := []string{
		"https://ks3.example.test",
		"ks3.example.test/storage",
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
