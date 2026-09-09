package ks3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	store "github.com/cocosip/sharp-store"
	ks3aws "github.com/ks3sdklib/aws-sdk-go/aws"
	"github.com/ks3sdklib/aws-sdk-go/aws/awserr"
	"github.com/ks3sdklib/aws-sdk-go/aws/credentials"
	ks3sdk "github.com/ks3sdklib/aws-sdk-go/service/s3"
)

const (
	Name                          = "ks3"
	BucketKey                     = "bucket"
	EndpointKey                   = "endpoint"
	AccessKeyKey                  = "access_key"
	SecretKeyKey                  = "secret_key"
	ProtocolKey                   = "protocol"
	UserAgentKey                  = "user_agent"
	MaxConnectionsKey             = "max_connections"
	TimeoutKey                    = "timeout"
	CreateContainerIfNotExistsKey = "create_container_if_not_exists"
)

const defaultAccessURLExpiry = time.Hour
const defaultClientRegion = "BEIJING"

type backendConfig struct {
	bucket                     string
	endpoint                   string
	accessKey                  string
	secretKey                  string
	protocol                   string
	userAgent                  string
	maxConnections             int
	timeout                    int
	createContainerIfNotExists bool
}

type Backend struct {
	clients sync.Map
}

func New() store.Backend {
	return &Backend{}
}

func (*Backend) Name() string {
	return Name
}

func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "KS3 bucket name."},
		{Name: EndpointKey, Type: "string", Required: true, Example: "ks3-cn-beijing.ksyuncs.com", Description: "KS3 host or IP with an optional port, without a URL scheme."},
		{Name: AccessKeyKey, Type: "string", Required: true, Sensitive: true, Description: "KS3 access key."},
		{Name: SecretKeyKey, Type: "string", Required: true, Sensitive: true, Description: "KS3 secret key."},
		{Name: ProtocolKey, Type: "string", Example: "https", Description: "Protocol for the configured endpoint: http or https. Defaults to http."},
		{Name: UserAgentKey, Type: "string", Example: "sharp-store", Description: "Optional User-Agent sent by the KS3 SDK."},
		{Name: MaxConnectionsKey, Type: "int", Example: "30", Description: "Optional maximum connections per KS3 endpoint."},
		{Name: TimeoutKey, Type: "int", Example: "100000", Description: "Optional whole-request timeout in milliseconds."},
		{Name: CreateContainerIfNotExistsKey, Type: "bool", Example: "false", Description: "Create the KS3 bucket before saving when it does not exist."},
	}
}

func (*Backend) ValidateConfig(ctx context.Context, values map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := readConfig(values)
	return err
}

func (b *Backend) Save(ctx context.Context, request store.SaveRequest) (string, error) {
	if request.Body == nil {
		return "", errors.New("file body is required")
	}
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return "", err
	}
	if err := ensureContainer(ctx, client, cfg); err != nil {
		return "", err
	}
	_, err = client.UploadReaderWithContext(ctx, &ks3sdk.UploadReaderInput{
		Bucket:          ks3aws.String(cfg.bucket),
		Key:             ks3aws.String(request.Key),
		Body:            request.Body,
		ForbidOverwrite: ks3aws.Boolean(!request.Overwrite),
	})
	if err != nil {
		if !request.Overwrite && isConflict(err) {
			return "", fmt.Errorf("%w: %s", store.ErrFileExists, request.FileID)
		}
		return "", err
	}
	return request.FileID, nil
}

func (b *Backend) Delete(ctx context.Context, request store.FileRequest) (bool, error) {
	exists, err := b.Exists(ctx, request)
	if err != nil || !exists {
		return false, err
	}
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = client.DeleteObjectWithContext(ctx, &ks3sdk.DeleteObjectInput{
		Bucket: ks3aws.String(cfg.bucket),
		Key:    ks3aws.String(request.Key),
	})
	return err == nil, err
}

func (b *Backend) Exists(ctx context.Context, request store.FileRequest) (bool, error) {
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = client.HeadObjectWithContext(ctx, &ks3sdk.HeadObjectInput{
		Bucket: ks3aws.String(cfg.bucket),
		Key:    ks3aws.String(request.Key),
	})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (b *Backend) Download(ctx context.Context, request store.DownloadRequest) (bool, error) {
	reader, err := b.GetOrNil(ctx, request.FileRequest)
	if err != nil || reader == nil {
		return false, err
	}
	defer func() { _ = reader.Close() }()
	if err := os.MkdirAll(filepath.Dir(request.Destination), 0o755); err != nil {
		return false, err
	}
	file, err := os.OpenFile(request.Destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	if _, err := io.Copy(file, reader); err != nil {
		return false, err
	}
	return true, file.Sync()
}

func (b *Backend) GetOrNil(ctx context.Context, request store.FileRequest) (io.ReadCloser, error) {
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return nil, err
	}
	output, err := client.GetObjectWithContext(ctx, &ks3sdk.GetObjectInput{
		Bucket: ks3aws.String(cfg.bucket),
		Key:    ks3aws.String(request.Key),
	})
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return output.Body, nil
}

func (b *Backend) AccessURL(ctx context.Context, request store.AccessURLRequest) (string, error) {
	if request.CheckExists {
		exists, err := b.Exists(ctx, request.FileRequest)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", store.ErrFileNotFound
		}
	}
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return "", err
	}
	expiresIn := defaultAccessURLExpiry
	if request.ExpiresAt != nil {
		expiresIn = time.Until(*request.ExpiresAt)
		if expiresIn <= 0 {
			return "", fmt.Errorf("%w: access URL expiry is in the past", store.ErrInvalidConfig)
		}
	}
	return client.GeneratePresignedUrl(&ks3sdk.GeneratePresignedUrlInput{
		Bucket:     ks3aws.String(cfg.bucket),
		Key:        ks3aws.String(request.Key),
		HTTPMethod: ks3sdk.GET,
		Expires:    int64(math.Ceil(expiresIn.Seconds())),
	})
}

func (b *Backend) client(ctx context.Context, values map[string]string) (*ks3sdk.S3, backendConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, backendConfig{}, err
	}
	cfg, err := readConfig(values)
	if err != nil {
		return nil, backendConfig{}, err
	}
	if client, ok := b.clients.Load(cfg); ok {
		return client.(*ks3sdk.S3), cfg, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.maxConnections > 0 {
		transport.MaxConnsPerHost = cfg.maxConnections
		transport.MaxIdleConnsPerHost = cfg.maxConnections
	}
	httpClient := &http.Client{Transport: transport}
	if cfg.timeout > 0 {
		httpClient.Timeout = time.Duration(cfg.timeout) * time.Millisecond
	}
	client := ks3sdk.New(&ks3aws.Config{
		Credentials:      credentials.NewStaticCredentials(cfg.accessKey, cfg.secretKey, ""),
		Endpoint:         cfg.endpoint,
		Region:           defaultClientRegion,
		DisableSSL:       cfg.protocol == "http",
		HTTPClient:       httpClient,
		S3ForcePathStyle: true,
		SignerVersion:    "V2",
	})
	if cfg.userAgent != "" {
		client.Handlers.Build.PushBack(func(request *ks3aws.Request) {
			request.HTTPRequest.Header.Set("User-Agent", cfg.userAgent)
		})
	}
	actual, loaded := b.clients.LoadOrStore(cfg, client)
	if loaded {
		client = actual.(*ks3sdk.S3)
	}
	return client, cfg, nil
}

func readConfig(values map[string]string) (backendConfig, error) {
	cfg := backendConfig{
		bucket:    values[BucketKey],
		endpoint:  values[EndpointKey],
		accessKey: values[AccessKeyKey],
		secretKey: values[SecretKeyKey],
		protocol:  values[ProtocolKey],
		userAgent: values[UserAgentKey],
	}
	for key, value := range map[string]string{
		BucketKey: cfg.bucket, EndpointKey: cfg.endpoint,
		AccessKeyKey: cfg.accessKey, SecretKeyKey: cfg.secretKey,
	} {
		if value == "" {
			return backendConfig{}, errors.New(key + " is required")
		}
	}
	if err := validateEndpoint(cfg.endpoint); err != nil {
		return backendConfig{}, err
	}
	if cfg.protocol == "" {
		cfg.protocol = "http"
	}
	if cfg.protocol != "http" && cfg.protocol != "https" {
		return backendConfig{}, errors.New("protocol must be http or https")
	}
	if value := values[MaxConnectionsKey]; value != "" {
		maxConnections, err := strconv.Atoi(value)
		if err != nil || maxConnections < 0 {
			return backendConfig{}, errors.New("max_connections must be a non-negative integer")
		}
		cfg.maxConnections = maxConnections
	}
	if value := values[TimeoutKey]; value != "" {
		timeout, err := strconv.Atoi(value)
		if err != nil || timeout < 0 {
			return backendConfig{}, errors.New("timeout must be a non-negative integer")
		}
		cfg.timeout = timeout
	}
	if value := values[CreateContainerIfNotExistsKey]; value != "" {
		create, err := strconv.ParseBool(value)
		if err != nil {
			return backendConfig{}, errors.New("create_container_if_not_exists must be a boolean")
		}
		cfg.createContainerIfNotExists = create
	}
	return cfg, nil
}

func ensureContainer(ctx context.Context, client *ks3sdk.S3, cfg backendConfig) error {
	if !cfg.createContainerIfNotExists {
		return nil
	}
	_, err := client.HeadBucketWithContext(ctx, &ks3sdk.HeadBucketInput{Bucket: ks3aws.String(cfg.bucket)})
	if err == nil {
		return nil
	}
	if !isNotFound(err) {
		return err
	}
	_, err = client.CreateBucketWithContext(ctx, &ks3sdk.CreateBucketInput{Bucket: ks3aws.String(cfg.bucket)})
	if !isBucketCreateConflict(err) {
		return err
	}
	_, err = client.HeadBucketWithContext(ctx, &ks3sdk.HeadBucketInput{Bucket: ks3aws.String(cfg.bucket)})
	return err
}

func validateEndpoint(endpoint string) error {
	if endpoint == "" {
		return errors.New("endpoint is required")
	}
	if endpoint != strings.TrimSpace(endpoint) || strings.Contains(endpoint, "://") {
		return errors.New("endpoint must be a host or host:port without a URL scheme")
	}
	parsed, err := url.Parse("http://" + endpoint)
	if err != nil || parsed.Host != endpoint || parsed.Hostname() == "" {
		return errors.New("endpoint must be a host or host:port without a URL scheme")
	}
	return nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var requestFailure awserr.RequestFailure
	if errors.As(err, &requestFailure) && requestFailure.StatusCode() == http.StatusNotFound {
		return true
	}
	var serviceError awserr.Error
	return errors.As(err, &serviceError) && (serviceError.Code() == ks3sdk.ErrCodeNoSuchKey || serviceError.Code() == "NoSuchObject" || serviceError.Code() == "NotFound")
}

func isConflict(err error) bool {
	var serviceError awserr.Error
	return errors.As(err, &serviceError) && (serviceError.Code() == "FileAlreadyExists" || serviceError.Code() == "ObjectAlreadyExists" || serviceError.Code() == "PreconditionFailed" || serviceError.Code() == "ConditionalRequestConflict")
}

func isBucketCreateConflict(err error) bool {
	var serviceError awserr.Error
	return errors.As(err, &serviceError) && (serviceError.Code() == "BucketAlreadyOwnedByYou" || serviceError.Code() == "BucketAlreadyExists")
}
