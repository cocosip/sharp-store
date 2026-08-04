package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	store "github.com/cocosip/sharp-store"
)

const (
	Name               = "s3"
	BucketKey          = "bucket"
	RegionKey          = "region"
	EndpointKey        = "endpoint"
	AccessKeyIDKey     = "access_key_id"
	SecretAccessKeyKey = "secret_access_key"
	SessionTokenKey    = "session_token"
	PathStyleKey       = "path_style"
)

type config struct {
	bucket          string
	region          string
	endpoint        string
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
	pathStyle       bool
}

type Backend struct {
	clients sync.Map
}

func New() *Backend {
	return &Backend{}
}

func (*Backend) Name() string {
	return Name
}

func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "S3 bucket name."},
		{Name: RegionKey, Type: "string", Example: "us-east-1", Description: "AWS region. Defaults to us-east-1."},
		{Name: EndpointKey, Type: "string", Example: "https://minio.example.test", Description: "Optional S3-compatible endpoint."},
		{Name: AccessKeyIDKey, Type: "string", Sensitive: true, Description: "Optional static access key ID."},
		{Name: SecretAccessKeyKey, Type: "string", Sensitive: true, Description: "Optional static secret access key."},
		{Name: SessionTokenKey, Type: "string", Sensitive: true, Description: "Optional session token."},
		{Name: PathStyleKey, Type: "bool", Example: "true", Description: "Use path-style bucket addressing."},
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
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return "", err
	}
	input := &awss3.PutObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key), Body: request.Body}
	if !request.Overwrite {
		input.IfNoneMatch = aws.String("*")
	}
	_, err = client.PutObject(ctx, input)
	if err != nil {
		if isConflict(err) {
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
	_, err = client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key)})
	return err == nil, err
}

func (b *Backend) Exists(ctx context.Context, request store.FileRequest) (bool, error) {
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key)})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, err
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
	output, err := client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key)})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
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
	presigner := awss3.NewPresignClient(client)
	options := []func(*awss3.PresignOptions){}
	if request.ExpiresAt != nil {
		expiresIn := time.Until(*request.ExpiresAt)
		if expiresIn <= 0 {
			return "", fmt.Errorf("%w: access URL expiry is in the past", store.ErrInvalidConfig)
		}
		options = append(options, func(options *awss3.PresignOptions) { options.Expires = expiresIn })
	}
	presigned, err := presigner.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(cfg.bucket),
		Key:    aws.String(request.Key),
	}, options...)
	if err != nil {
		return "", err
	}
	return presigned.URL, nil
}

func (b *Backend) client(ctx context.Context, values map[string]string) (*awss3.Client, config, error) {
	cfg, err := readConfig(values)
	if err != nil {
		return nil, config{}, err
	}
	if client, ok := b.clients.Load(cfg); ok {
		return client.(*awss3.Client), cfg, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.region))
	if err != nil {
		return nil, config{}, err
	}
	awsCfg.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	if cfg.accessKeyID != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider(cfg.accessKeyID, cfg.secretAccessKey, cfg.sessionToken)
	}
	client := awss3.NewFromConfig(awsCfg, func(options *awss3.Options) {
		options.UsePathStyle = cfg.pathStyle
		if cfg.endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.endpoint)
		}
		if cfg.usesHTTP() {
			options.APIOptions = append(options.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
		}
	})
	actual, loaded := b.clients.LoadOrStore(cfg, client)
	if loaded {
		return actual.(*awss3.Client), cfg, nil
	}
	return client, cfg, nil
}

func (cfg config) usesHTTP() bool {
	endpoint, err := url.Parse(cfg.endpoint)
	return err == nil && endpoint.Scheme == "http"
}

func readConfig(values map[string]string) (config, error) {
	cfg := config{
		bucket:          values[BucketKey],
		region:          values[RegionKey],
		endpoint:        values[EndpointKey],
		accessKeyID:     values[AccessKeyIDKey],
		secretAccessKey: values[SecretAccessKeyKey],
		sessionToken:    values[SessionTokenKey],
	}
	if cfg.bucket == "" {
		return config{}, errors.New("bucket is required")
	}
	if cfg.region == "" {
		cfg.region = "us-east-1"
	}
	if cfg.endpoint != "" {
		endpoint, err := url.ParseRequestURI(cfg.endpoint)
		if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
			return config{}, errors.New("endpoint must be an absolute URL")
		}
	}
	if (cfg.accessKeyID == "") != (cfg.secretAccessKey == "") {
		return config{}, errors.New("access_key_id and secret_access_key must be configured together")
	}
	if value := values[PathStyleKey]; value != "" {
		pathStyle, err := strconv.ParseBool(value)
		if err != nil {
			return config{}, errors.New("path_style must be a boolean")
		}
		cfg.pathStyle = pathStyle
	}
	return cfg, nil
}

func isNotFound(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "NoSuchKey", "NoSuchObject", "NotFound", "404":
			return true
		}
	}
	return false
}

func isConflict(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "PreconditionFailed", "ConditionalRequestConflict", "412", "409":
			return true
		}
	}
	return false
}
