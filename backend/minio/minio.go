package minio

import (
	"errors"
	"net/url"
	"strings"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/s3"
	"github.com/cocosip/sharp-store/backend/s3compat"
)

const (
	Name         = "minio"
	BucketKey    = "bucket"
	EndpointKey  = "endpoint"
	AccessKeyKey = "access_key"
	SecretKeyKey = "secret_key"
	UseSSLKey    = "use_ssl"
	RegionKey    = "region"
)

func New() store.Backend {
	return s3compat.New(configTranslator{})
}

type configTranslator struct{}

func (configTranslator) Name() string {
	return Name
}

func (configTranslator) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "MinIO bucket name."},
		{Name: EndpointKey, Type: "string", Required: true, Example: "minio.example.test:9000", Description: "MinIO endpoint, with or without a URL scheme."},
		{Name: AccessKeyKey, Type: "string", Required: true, Sensitive: true, Description: "MinIO access key."},
		{Name: SecretKeyKey, Type: "string", Required: true, Sensitive: true, Description: "MinIO secret key."},
		{Name: UseSSLKey, Type: "bool", Example: "true", Description: "Use HTTPS for an endpoint without a scheme."},
		{Name: RegionKey, Type: "string", Example: "us-east-1", Description: "Signing region. Defaults to us-east-1."},
	}
}

func (configTranslator) Translate(values map[string]string) (map[string]string, error) {
	endpoint, err := endpointURL(values[EndpointKey], values[UseSSLKey])
	if err != nil {
		return nil, err
	}
	return map[string]string{
		s3.BucketKey:          values[BucketKey],
		s3.RegionKey:          values[RegionKey],
		s3.EndpointKey:        endpoint,
		s3.AccessKeyIDKey:     values[AccessKeyKey],
		s3.SecretAccessKeyKey: values[SecretKeyKey],
		s3.PathStyleKey:       "true",
	}, nil
}

func endpointURL(endpoint, useSSL string) (string, error) {
	if endpoint == "" {
		return "", errors.New("endpoint is required")
	}
	if strings.Contains(endpoint, "://") {
		parsed, err := url.ParseRequestURI(endpoint)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", errors.New("endpoint must be an absolute URL")
		}
		return endpoint, nil
	}
	if useSSL == "true" {
		return "https://" + endpoint, nil
	}
	if useSSL != "" && useSSL != "false" {
		return "", errors.New("use_ssl must be a boolean")
	}
	return "http://" + endpoint, nil
}
