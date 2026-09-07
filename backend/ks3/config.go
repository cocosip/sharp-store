package ks3

import (
	"fmt"

	store "github.com/cocosip/sharp-store"
)

type Config struct{ Bucket, Endpoint, AccessKey, SecretKey, Protocol, Region string }

func NewConfig() Config { return Config{} }

func (c Config) WithBucket(bucket string) Config     { c.Bucket = bucket; return c }
func (c Config) WithEndpoint(endpoint string) Config { c.Endpoint = endpoint; return c }
func (c Config) WithCredentials(accessKey, secretKey string) Config {
	c.AccessKey = accessKey
	c.SecretKey = secretKey
	return c
}
func (c Config) WithProtocol(protocol string) Config { c.Protocol = protocol; return c }
func (c Config) WithRegion(region string) Config     { c.Region = region; return c }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{BucketKey: c.Bucket, EndpointKey: c.Endpoint, AccessKeyKey: c.AccessKey, SecretKeyKey: c.SecretKey, ProtocolKey: c.Protocol, RegionKey: c.Region}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	return Config{Bucket: config.Values[BucketKey], Endpoint: config.Values[EndpointKey], AccessKey: config.Values[AccessKeyKey], SecretKey: config.Values[SecretKeyKey], Protocol: config.Values[ProtocolKey], Region: config.Values[RegionKey]}, nil
}
