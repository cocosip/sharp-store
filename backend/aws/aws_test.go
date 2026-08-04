package aws

import (
	"testing"

	"github.com/cocosip/sharp-store/backend/s3"
)

func TestConfigTranslatorPreservesAWSConnectionSettings(t *testing.T) {
	values, err := (configTranslator{}).Translate(map[string]string{
		BucketKey:          "archive",
		RegionKey:          "ap-southeast-1",
		EndpointKey:        "https://s3.example.test",
		AccessKeyIDKey:     "access",
		SecretAccessKeyKey: "secret",
		SessionTokenKey:    "token",
		PathStyleKey:       "true",
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got, want := values[s3.SessionTokenKey], "token"; got != want {
		t.Fatalf("session token = %q, want %q", got, want)
	}
	if got, want := values[s3.PathStyleKey], "true"; got != want {
		t.Fatalf("path style = %q, want %q", got, want)
	}
}
