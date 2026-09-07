package minio

import (
	"fmt"
	"strconv"

	store "github.com/cocosip/sharp-store"
)

type Config struct {
	Bucket, Endpoint, AccessKey, SecretKey, Region string
	UseSSL                                         bool
}

func NewConfig() Config { return Config{} }

func (c Config) WithBucket(bucket string) Config     { c.Bucket = bucket; return c }
func (c Config) WithEndpoint(endpoint string) Config { c.Endpoint = endpoint; return c }
func (c Config) WithCredentials(accessKey, secretKey string) Config {
	c.AccessKey = accessKey
	c.SecretKey = secretKey
	return c
}
func (c Config) WithRegion(region string) Config { c.Region = region; return c }
func (c Config) WithSSL(useSSL bool) Config      { c.UseSSL = useSSL; return c }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{BucketKey: c.Bucket, EndpointKey: c.Endpoint, AccessKeyKey: c.AccessKey, SecretKeyKey: c.SecretKey, RegionKey: c.Region, UseSSLKey: strconv.FormatBool(c.UseSSL)}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	useSSL := false
	if raw := config.Values[UseSSLKey]; raw != "" {
		var err error
		useSSL, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %s: %v", store.ErrInvalidConfig, UseSSLKey, err)
		}
	}
	return Config{Bucket: config.Values[BucketKey], Endpoint: config.Values[EndpointKey], AccessKey: config.Values[AccessKeyKey], SecretKey: config.Values[SecretKeyKey], Region: config.Values[RegionKey], UseSSL: useSSL}, nil
}
