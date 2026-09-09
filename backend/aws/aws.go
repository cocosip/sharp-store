package aws

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
	awstypes "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	store "github.com/cocosip/sharp-store"
)

const (
	Name                          = "aws"
	BucketKey                     = "bucket"
	RegionKey                     = "region"
	AccessKeyIDKey                = "access_key_id"
	SecretAccessKeyKey            = "secret_access_key"
	SessionTokenKey               = "session_token"
	CreateContainerIfNotExistsKey = "create_container_if_not_exists"
)

type backendConfig struct {
	bucket                     string
	region                     string
	accessKeyID                string
	secretAccessKey            string
	sessionToken               string
	createContainerIfNotExists bool
}

type Backend struct {
	clients        sync.Map
	baseEndpoint   string
	forcePathStyle bool
}

func New() store.Backend      { return &Backend{} }
func (*Backend) Name() string { return Name }

func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "AWS S3 bucket name."},
		{Name: RegionKey, Type: "string", Example: "us-east-1", Description: "AWS region. Defaults to us-east-1."},
		{Name: AccessKeyIDKey, Type: "string", Sensitive: true, Description: "Optional static access key ID; omit it to use the AWS default credential chain."},
		{Name: SecretAccessKeyKey, Type: "string", Sensitive: true, Description: "Optional static secret access key."},
		{Name: SessionTokenKey, Type: "string", Sensitive: true, Description: "Optional session token for static temporary credentials."},
		{Name: CreateContainerIfNotExistsKey, Type: "bool", Example: "false", Description: "Create the S3 bucket before saving when it does not exist."},
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
	input := &awss3.PutObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key), Body: request.Body}
	if !request.Overwrite {
		input.IfNoneMatch = aws.String("*")
	}
	_, err = client.PutObject(ctx, input)
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
	_, err = client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key)})
	return err == nil, err
}

func (b *Backend) Exists(ctx context.Context, request store.FileRequest) (bool, error) {
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key)})
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
	output, err := client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key)})
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
	presignOptions := []func(*awss3.PresignOptions){}
	if request.ExpiresAt != nil {
		expiresIn := time.Until(*request.ExpiresAt)
		if expiresIn <= 0 {
			return "", fmt.Errorf("%w: access URL expiry is in the past", store.ErrInvalidConfig)
		}
		presignOptions = append(presignOptions, func(options *awss3.PresignOptions) { options.Expires = expiresIn })
	}
	presigned, err := awss3.NewPresignClient(client).PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key),
	}, presignOptions...)
	if err != nil {
		return "", err
	}
	return presigned.URL, nil
}

func (b *Backend) client(ctx context.Context, values map[string]string) (*awss3.Client, backendConfig, error) {
	cfg, err := readConfig(values)
	if err != nil {
		return nil, backendConfig{}, err
	}
	if cached, ok := b.clients.Load(cfg); ok {
		return cached.(*awss3.Client), cfg, nil
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.region))
	if err != nil {
		return nil, backendConfig{}, err
	}
	awsConfig.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	if cfg.accessKeyID != "" {
		awsConfig.Credentials = credentials.NewStaticCredentialsProvider(cfg.accessKeyID, cfg.secretAccessKey, cfg.sessionToken)
	}
	client := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		if b.baseEndpoint != "" {
			options.BaseEndpoint = aws.String(b.baseEndpoint)
			options.UsePathStyle = b.forcePathStyle
			if usesHTTP(b.baseEndpoint) {
				options.APIOptions = append(options.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
			}
		}
	})
	actual, loaded := b.clients.LoadOrStore(cfg, client)
	if loaded {
		client = actual.(*awss3.Client)
	}
	return client, cfg, nil
}

func readConfig(values map[string]string) (backendConfig, error) {
	cfg := backendConfig{
		bucket: values[BucketKey], region: values[RegionKey],
		accessKeyID: values[AccessKeyIDKey], secretAccessKey: values[SecretAccessKeyKey],
		sessionToken: values[SessionTokenKey],
	}
	if cfg.bucket == "" {
		return backendConfig{}, errors.New("bucket is required")
	}
	if cfg.region == "" {
		cfg.region = "us-east-1"
	}
	if (cfg.accessKeyID == "") != (cfg.secretAccessKey == "") {
		return backendConfig{}, errors.New("access_key_id and secret_access_key must be configured together")
	}
	if cfg.sessionToken != "" && cfg.accessKeyID == "" {
		return backendConfig{}, errors.New("session_token requires static access_key_id and secret_access_key")
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

func ensureContainer(ctx context.Context, client *awss3.Client, cfg backendConfig) error {
	if !cfg.createContainerIfNotExists {
		return nil
	}
	_, err := client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(cfg.bucket)})
	if err == nil {
		return nil
	}
	if !isNotFound(err) {
		return err
	}
	input := &awss3.CreateBucketInput{Bucket: aws.String(cfg.bucket)}
	if cfg.region != "us-east-1" {
		input.CreateBucketConfiguration = &awstypes.CreateBucketConfiguration{
			LocationConstraint: awstypes.BucketLocationConstraint(cfg.region),
		}
	}
	_, err = client.CreateBucket(ctx, input)
	if !isBucketCreateConflict(err) {
		return err
	}
	_, err = client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(cfg.bucket)})
	return err
}

func usesHTTP(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	return err == nil && parsed.Scheme == "http"
}

func isNotFound(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "NoSuchBucket", "NoSuchKey", "NoSuchObject", "NotFound", "404":
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

func isBucketCreateConflict(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && (apiError.ErrorCode() == "BucketAlreadyOwnedByYou" || apiError.ErrorCode() == "BucketAlreadyExists")
}
