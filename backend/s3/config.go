package s3

import (
	"fmt"
	"strconv"

	store "github.com/cocosip/sharp-store"
)

type Config struct {
	Bucket                  string
	BaseEndpoint            string
	AccessKeyID             string
	SecretAccessKey         string
	ForcePathStyle          bool
	UseChunkEncoding        bool
	Protocol                string
	CreateBucketIfNotExists bool
}

func NewConfig() Config { return Config{} }

func (c Config) WithBucket(bucket string) Config         { c.Bucket = bucket; return c }
func (c Config) WithBaseEndpoint(endpoint string) Config { c.BaseEndpoint = endpoint; return c }
func (c Config) WithCredentials(accessKey, secretKey string) Config {
	c.AccessKeyID = accessKey
	c.SecretAccessKey = secretKey
	return c
}
func (c Config) WithForcePathStyle(force bool) Config { c.ForcePathStyle = force; return c }
func (c Config) WithUseChunkEncoding(use bool) Config { c.UseChunkEncoding = use; return c }
func (c Config) WithProtocol(protocol string) Config {
	c.Protocol = protocol
	return c
}
func (c Config) WithCreateBucketIfNotExists(create bool) Config {
	c.CreateBucketIfNotExists = create
	return c
}

func (Config) BackendName() string { return Name }

func (c Config) Values() map[string]string {
	return map[string]string{
		BucketKey: c.Bucket, BaseEndpointKey: c.BaseEndpoint,
		AccessKeyIDKey: c.AccessKeyID, SecretAccessKeyKey: c.SecretAccessKey,
		ForcePathStyleKey:          strconv.FormatBool(c.ForcePathStyle),
		UseChunkEncodingKey:        strconv.FormatBool(c.UseChunkEncoding),
		ProtocolKey:                c.Protocol,
		CreateBucketIfNotExistsKey: strconv.FormatBool(c.CreateBucketIfNotExists),
	}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	forcePathStyle, err := parseBool(config.Values, ForcePathStyleKey)
	if err != nil {
		return Config{}, err
	}
	useChunkEncoding, err := parseBool(config.Values, UseChunkEncodingKey)
	if err != nil {
		return Config{}, err
	}
	createBucket, err := parseBool(config.Values, CreateBucketIfNotExistsKey)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Bucket: config.Values[BucketKey], BaseEndpoint: config.Values[BaseEndpointKey],
		AccessKeyID: config.Values[AccessKeyIDKey], SecretAccessKey: config.Values[SecretAccessKeyKey],
		ForcePathStyle: forcePathStyle, UseChunkEncoding: useChunkEncoding,
		Protocol:                config.Values[ProtocolKey],
		CreateBucketIfNotExists: createBucket,
	}, nil
}

func parseBool(values map[string]string, key string) (bool, error) {
	if values[key] == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(values[key])
	if err != nil {
		return false, fmt.Errorf("%w: %s: %v", store.ErrInvalidConfig, key, err)
	}
	return value, nil
}
