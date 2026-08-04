package aws

import (
	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/s3"
	"github.com/cocosip/sharp-store/backend/s3compat"
)

const (
	Name               = "aws"
	BucketKey          = "bucket"
	RegionKey          = "region"
	EndpointKey        = "endpoint"
	AccessKeyIDKey     = "access_key_id"
	SecretAccessKeyKey = "secret_access_key"
	SessionTokenKey    = "session_token"
	PathStyleKey       = "path_style"
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
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "S3 bucket name."},
		{Name: RegionKey, Type: "string", Example: "us-east-1", Description: "AWS region. Defaults to us-east-1."},
		{Name: EndpointKey, Type: "string", Example: "https://s3.us-east-1.amazonaws.com", Description: "Optional endpoint for AWS-compatible testing or private partitions."},
		{Name: AccessKeyIDKey, Type: "string", Sensitive: true, Description: "Optional static access key ID."},
		{Name: SecretAccessKeyKey, Type: "string", Sensitive: true, Description: "Optional static secret access key."},
		{Name: SessionTokenKey, Type: "string", Sensitive: true, Description: "Optional session token."},
		{Name: PathStyleKey, Type: "bool", Example: "true", Description: "Use path-style bucket addressing."},
	}
}

func (configTranslator) Translate(values map[string]string) (map[string]string, error) {
	return map[string]string{
		s3.BucketKey:          values[BucketKey],
		s3.RegionKey:          values[RegionKey],
		s3.EndpointKey:        values[EndpointKey],
		s3.AccessKeyIDKey:     values[AccessKeyIDKey],
		s3.SecretAccessKeyKey: values[SecretAccessKeyKey],
		s3.SessionTokenKey:    values[SessionTokenKey],
		s3.PathStyleKey:       values[PathStyleKey],
	}, nil
}
