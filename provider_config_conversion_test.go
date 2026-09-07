package store_test

import (
	"testing"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/aliyun"
	"github.com/cocosip/sharp-store/backend/aws"
	"github.com/cocosip/sharp-store/backend/azure"
	"github.com/cocosip/sharp-store/backend/filesystem"
	"github.com/cocosip/sharp-store/backend/ks3"
	"github.com/cocosip/sharp-store/backend/minio"
	"github.com/cocosip/sharp-store/backend/obs"
	"github.com/cocosip/sharp-store/backend/s3"
)

func TestProviderConfigsRoundTripThroughContainerConfig(t *testing.T) {
	t.Run("filesystem", func(t *testing.T) {
		generic := store.NewContainerConfig(
			filesystem.NewConfig().WithRoot("D:/files").WithBaseURL("https://files.example"),
		)
		config, err := filesystem.ParseConfig(generic)
		if err != nil || config.Root != "D:/files" || config.BaseURL != "https://files.example" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})

	t.Run("s3", func(t *testing.T) {
		generic := store.NewContainerConfig(
			s3.NewConfig().
				WithBucket("images").
				WithRegion("us-east-1").
				WithEndpoint("https://s3.example").
				WithCredentials("access", "secret").
				WithSessionToken("session").
				WithPathStyle(true),
		)
		config, err := s3.ParseConfig(generic)
		if err != nil || config.Bucket != "images" || !config.PathStyle || config.SessionToken != "session" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})

	t.Run("aws", func(t *testing.T) {
		generic := store.NewContainerConfig(
			aws.NewConfig().
				WithBucket("archive").
				WithRegion("cn-north-1").
				WithEndpoint("https://aws.example").
				WithCredentials("access", "secret").
				WithSessionToken("session").
				WithPathStyle(true),
		)
		config, err := aws.ParseConfig(generic)
		if err != nil || config.Bucket != "archive" || !config.PathStyle || config.Region != "cn-north-1" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})

	t.Run("minio", func(t *testing.T) {
		generic := store.NewContainerConfig(
			minio.NewConfig().
				WithBucket("dicom").
				WithEndpoint("minio:9000").
				WithCredentials("access", "secret").
				WithRegion("us-east-1").
				WithSSL(true),
		)
		config, err := minio.ParseConfig(generic)
		if err != nil || config.Endpoint != "minio:9000" || !config.UseSSL || config.AccessKey != "access" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})

	t.Run("ks3", func(t *testing.T) {
		generic := store.NewContainerConfig(
			ks3.NewConfig().
				WithBucket("dicom").
				WithEndpoint("ks3.example").
				WithCredentials("access", "secret").
				WithProtocol("https").
				WithRegion("BEIJING"),
		)
		config, err := ks3.ParseConfig(generic)
		if err != nil || config.Protocol != "https" || config.Region != "BEIJING" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})

	t.Run("azure", func(t *testing.T) {
		generic := store.NewContainerConfig(
			azure.NewConfig().WithConnectionString("connection").WithContainer("images"),
		)
		config, err := azure.ParseConfig(generic)
		if err != nil || config.ConnectionString != "connection" || config.Container != "images" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})

	t.Run("aliyun", func(t *testing.T) {
		generic := store.NewContainerConfig(
			aliyun.NewConfig().
				WithEndpoint("oss.example").
				WithBucket("images").
				WithCredentials("access", "secret"),
		)
		config, err := aliyun.ParseConfig(generic)
		if err != nil || config.AccessKeyID != "access" || config.Bucket != "images" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})

	t.Run("obs", func(t *testing.T) {
		generic := store.NewContainerConfig(
			obs.NewConfig().
				WithEndpoint("obs.example").
				WithBucket("images").
				WithCredentials("access", "secret"),
		)
		config, err := obs.ParseConfig(generic)
		if err != nil || config.AccessKeySecret != "secret" || config.Endpoint != "obs.example" {
			t.Fatalf("ParseConfig() = (%#v, %v)", config, err)
		}
	})
}

func TestProviderParseConfigRejectsAnotherBackend(t *testing.T) {
	_, err := minio.ParseConfig(store.ContainerConfig{Backend: filesystem.Name})
	if err == nil {
		t.Fatal("ParseConfig() error = nil, want backend mismatch")
	}
}

func TestProviderParseConfigUsesFalseForMissingOptionalBooleans(t *testing.T) {
	tests := []struct {
		name  string
		parse func() (bool, error)
	}{
		{
			name: "s3 path style",
			parse: func() (bool, error) {
				config, err := s3.ParseConfig(store.ContainerConfig{Backend: s3.Name})
				return config.PathStyle, err
			},
		},
		{
			name: "aws path style",
			parse: func() (bool, error) {
				config, err := aws.ParseConfig(store.ContainerConfig{Backend: aws.Name})
				return config.PathStyle, err
			},
		},
		{
			name: "minio ssl",
			parse: func() (bool, error) {
				config, err := minio.ParseConfig(store.ContainerConfig{Backend: minio.Name})
				return config.UseSSL, err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.parse()
			if err != nil || value {
				t.Fatalf("ParseConfig() = (%t, %v), want (false, nil)", value, err)
			}
		})
	}
}

func TestContainerConfigSupportsTenantModeChaining(t *testing.T) {
	config := store.NewContainerConfig(filesystem.NewConfig().WithRoot("D:/files")).
		WithTenantMode(store.TenantShared)
	if config.TenantMode != store.TenantShared {
		t.Fatalf("TenantMode = %d, want TenantShared", config.TenantMode)
	}
}
