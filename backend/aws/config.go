package aws

import (
	"fmt"
	"strconv"

	store "github.com/cocosip/sharp-store"
)

type Config struct {
	Bucket, Region, AccessKeyID, SecretAccessKey, SessionToken string
	CreateContainerIfNotExists                                 bool
}

func NewConfig() Config { return Config{} }

func (c Config) WithBucket(bucket string) Config { c.Bucket = bucket; return c }
func (c Config) WithRegion(region string) Config { c.Region = region; return c }
func (c Config) WithCredentials(accessKey, secretKey string) Config {
	c.AccessKeyID = accessKey
	c.SecretAccessKey = secretKey
	return c
}
func (c Config) WithSessionToken(token string) Config { c.SessionToken = token; return c }
func (c Config) WithCreateContainerIfNotExists(create bool) Config {
	c.CreateContainerIfNotExists = create
	return c
}

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{
		BucketKey: c.Bucket, RegionKey: c.Region,
		AccessKeyIDKey: c.AccessKeyID, SecretAccessKeyKey: c.SecretAccessKey,
		SessionTokenKey:               c.SessionToken,
		CreateContainerIfNotExistsKey: strconv.FormatBool(c.CreateContainerIfNotExists),
	}
}

func ParseConfig(config store.ContainerConfig) (Config, error) {
	if config.Backend != Name {
		return Config{}, fmt.Errorf("%w: expected backend %q, got %q", store.ErrInvalidConfig, Name, config.Backend)
	}
	createContainer := false
	if raw := config.Values[CreateContainerIfNotExistsKey]; raw != "" {
		var err error
		createContainer, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %s: %v", store.ErrInvalidConfig, CreateContainerIfNotExistsKey, err)
		}
	}
	return Config{
		Bucket: config.Values[BucketKey], Region: config.Values[RegionKey],
		AccessKeyID: config.Values[AccessKeyIDKey], SecretAccessKey: config.Values[SecretAccessKeyKey],
		SessionToken: config.Values[SessionTokenKey], CreateContainerIfNotExists: createContainer,
	}, nil
}
