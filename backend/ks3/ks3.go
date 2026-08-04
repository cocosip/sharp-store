package ks3

import (
	"errors"
	"net/url"
	"strings"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/s3"
	"github.com/cocosip/sharp-store/backend/s3compat"
)

const (
	Name         = "ks3"
	BucketKey    = "bucket"
	EndpointKey  = "endpoint"
	AccessKeyKey = "access_key"
	SecretKeyKey = "secret_key"
	ProtocolKey  = "protocol"
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
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "KS3 bucket name."},
		{Name: EndpointKey, Type: "string", Required: true, Example: "ks3-cn-beijing.ksyuncs.com", Description: "KS3 endpoint, with or without a URL scheme."},
		{Name: AccessKeyKey, Type: "string", Required: true, Sensitive: true, Description: "KS3 access key."},
		{Name: SecretKeyKey, Type: "string", Required: true, Sensitive: true, Description: "KS3 secret key."},
		{Name: ProtocolKey, Type: "string", Example: "https", Description: "Protocol for an endpoint without a scheme: http or https."},
		{Name: RegionKey, Type: "string", Example: "us-east-1", Description: "Signing region. Defaults to us-east-1."},
	}
}

func (configTranslator) Translate(values map[string]string) (map[string]string, error) {
	endpoint, err := endpointURL(values[EndpointKey], values[ProtocolKey])
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

func endpointURL(endpoint, protocol string) (string, error) {
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
	if protocol == "" {
		protocol = "https"
	}
	if protocol != "http" && protocol != "https" {
		return "", errors.New("protocol must be http or https")
	}
	return protocol + "://" + endpoint, nil
}
