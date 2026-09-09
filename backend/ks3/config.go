package ks3

import (
	"fmt"
	"strconv"

	store "github.com/cocosip/sharp-store"
)

type Config struct {
	Bucket                     string
	Endpoint                   string
	AccessKey                  string
	SecretKey                  string
	Protocol                   string
	UserAgent                  string
	MaxConnections             int
	Timeout                    int
	CreateContainerIfNotExists bool
}

func NewConfig() Config { return Config{} }

func (c Config) WithBucket(bucket string) Config     { c.Bucket = bucket; return c }
func (c Config) WithEndpoint(endpoint string) Config { c.Endpoint = endpoint; return c }
func (c Config) WithCredentials(accessKey, secretKey string) Config {
	c.AccessKey = accessKey
	c.SecretKey = secretKey
	return c
}
func (c Config) WithProtocol(protocol string) Config   { c.Protocol = protocol; return c }
func (c Config) WithUserAgent(userAgent string) Config { c.UserAgent = userAgent; return c }
func (c Config) WithMaxConnections(maxConnections int) Config {
	c.MaxConnections = maxConnections
	return c
}
func (c Config) WithTimeout(timeoutMilliseconds int) Config {
	c.Timeout = timeoutMilliseconds
	return c
}
func (c Config) WithCreateContainerIfNotExists(create bool) Config {
	c.CreateContainerIfNotExists = create
	return c
}

func (Config) BackendName() string { return Name }

func (c Config) Values() map[string]string {
	return map[string]string{
		BucketKey: c.Bucket, EndpointKey: c.Endpoint,
		AccessKeyKey: c.AccessKey, SecretKeyKey: c.SecretKey,
		ProtocolKey:                   c.Protocol,
		UserAgentKey:                  c.UserAgent,
		MaxConnectionsKey:             strconv.Itoa(c.MaxConnections),
		TimeoutKey:                    strconv.Itoa(c.Timeout),
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
	maxConnections, err := parseConfigInt(config.Values, MaxConnectionsKey)
	if err != nil {
		return Config{}, err
	}
	timeout, err := parseConfigInt(config.Values, TimeoutKey)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Bucket: config.Values[BucketKey], Endpoint: config.Values[EndpointKey],
		AccessKey: config.Values[AccessKeyKey], SecretKey: config.Values[SecretKeyKey],
		Protocol: config.Values[ProtocolKey], UserAgent: config.Values[UserAgentKey],
		MaxConnections: maxConnections, Timeout: timeout,
		CreateContainerIfNotExists: createContainer,
	}, nil
}

func parseConfigInt(values map[string]string, key string) (int, error) {
	if values[key] == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(values[key])
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %v", store.ErrInvalidConfig, key, err)
	}
	return value, nil
}
