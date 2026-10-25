package minio

import (
	"fmt"
	"strconv"

	store "github.com/cocosip/sharp-store"
)

type Config struct {
	Bucket                  string
	Endpoint                string
	AccessKey               string
	SecretKey               string
	Region                  string
	Secure                  bool
	CreateBucketIfNotExists bool
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
func (c Config) WithSecure(secure bool) Config   { c.Secure = secure; return c }
func (c Config) WithCreateBucketIfNotExists(create bool) Config {
	c.CreateBucketIfNotExists = create
	return c
}

func (Config) BackendName() string { return Name }

func (c Config) Values() map[string]string {
	return map[string]string{
		BucketKey: c.Bucket, EndpointKey: c.Endpoint,
		AccessKeyKey: c.AccessKey, SecretKeyKey: c.SecretKey,
		RegionKey:                  c.Region,
		SecureKey:                  strconv.FormatBool(c.Secure),
		CreateBucketIfNotExistsKey: strconv.FormatBool(c.CreateBucketIfNotExists),
	}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	secure, err := parseConfigBool(config.Values, SecureKey)
	if err != nil {
		return Config{}, err
	}
	createBucket, err := parseConfigBool(config.Values, CreateBucketIfNotExistsKey)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Bucket: config.Values[BucketKey], Endpoint: config.Values[EndpointKey],
		AccessKey: config.Values[AccessKeyKey], SecretKey: config.Values[SecretKeyKey],
		Region: config.Values[RegionKey], Secure: secure, CreateBucketIfNotExists: createBucket,
	}, nil
}

func parseConfigBool(values map[string]string, key string) (bool, error) {
	if values[key] == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(values[key])
	if err != nil {
		return false, fmt.Errorf("%w: %s: %v", store.ErrInvalidConfig, key, err)
	}
	return value, nil
}
