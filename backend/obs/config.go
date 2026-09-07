package obs

import (
	"fmt"

	store "github.com/cocosip/sharp-store"
)

type Config struct{ Endpoint, Bucket, AccessKeyID, AccessKeySecret string }

func NewConfig() Config { return Config{} }

func (c Config) WithEndpoint(endpoint string) Config { c.Endpoint = endpoint; return c }
func (c Config) WithBucket(bucket string) Config     { c.Bucket = bucket; return c }
func (c Config) WithCredentials(accessKeyID, accessKeySecret string) Config {
	c.AccessKeyID = accessKeyID
	c.AccessKeySecret = accessKeySecret
	return c
}

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{EndpointKey: c.Endpoint, BucketKey: c.Bucket, AccessKeyIDKey: c.AccessKeyID, AccessKeySecretKey: c.AccessKeySecret}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	return Config{Endpoint: config.Values[EndpointKey], Bucket: config.Values[BucketKey], AccessKeyID: config.Values[AccessKeyIDKey], AccessKeySecret: config.Values[AccessKeySecretKey]}, nil
}
