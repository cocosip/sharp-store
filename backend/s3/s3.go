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
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	store "github.com/cocosip/sharp-store"
)

const (
	Name                       = "s3"
	BucketKey                  = "bucket"
	BaseEndpointKey            = "base_endpoint"
	AccessKeyIDKey             = "access_key_id"
	SecretAccessKeyKey         = "secret_access_key"
	ForcePathStyleKey          = "force_path_style"
	UseChunkEncodingKey        = "use_chunk_encoding"
	ProtocolKey                = "protocol"
	CreateBucketIfNotExistsKey = "create_bucket_if_not_exists"
)

const defaultSigningRegion = "us-east-1"

type backendConfig struct {
	bucket                  string
	baseEndpoint            string
	accessKeyID             string
	secretAccessKey         string
	forcePathStyle          bool
	useChunkEncoding        bool
	protocol                string
	createBucketIfNotExists bool
}

type Backend struct{ clients sync.Map }

func New() store.Backend      { return &Backend{} }
func (*Backend) Name() string { return Name }

func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "S3 bucket name."},
		{Name: BaseEndpointKey, Type: "string", Required: true, Example: "https://s3.example.test", Description: "Absolute HTTP(S) base endpoint passed to the S3 SDK."},
		{Name: AccessKeyIDKey, Type: "string", Required: true, Sensitive: true, Description: "Static access key ID."},
		{Name: SecretAccessKeyKey, Type: "string", Required: true, Sensitive: true, Description: "Static secret access key."},
		{Name: ForcePathStyleKey, Type: "bool", Example: "true", Description: "Use path-style bucket addressing."},
		{Name: UseChunkEncodingKey, Type: "bool", Example: "false", Description: "Stream unknown-length uploads with HTTP chunked transfer encoding."},
		{Name: ProtocolKey, Type: "string", Example: "https", Description: "Protocol used in generated access URLs: http or https. Defaults to base_endpoint."},
		{Name: CreateBucketIfNotExistsKey, Type: "bool", Example: "false", Description: "Create the bucket before saving when it does not exist."},
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
	if err := ensureBucket(ctx, client, cfg); err != nil {
		return "", err
	}
	body := request.Body
	var contentLength *int64
	cleanup := func() {}
	if !cfg.useChunkEncoding {
		body, contentLength, cleanup, err = fixedLengthBody(ctx, request.Body)
		if err != nil {
			return "", err
		}
	}
	defer cleanup()
	input := &awss3.PutObjectInput{
		Bucket: aws.String(cfg.bucket), Key: aws.String(request.Key), Body: body,
		ContentLength: contentLength,
	}
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
	accessURL, err := url.Parse(presigned.URL)
	if err != nil {
		return "", err
	}
	accessURL.Scheme = cfg.protocol
	return accessURL.String(), nil
}

func (b *Backend) client(ctx context.Context, values map[string]string) (*awss3.Client, backendConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, backendConfig{}, err
	}
	cfg, err := readConfig(values)
	if err != nil {
		return nil, backendConfig{}, err
	}
	if cached, ok := b.clients.Load(cfg); ok {
		return cached.(*awss3.Client), cfg, nil
	}
	awsConfig := aws.Config{
		Region:      defaultSigningRegion,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(cfg.accessKeyID, cfg.secretAccessKey, "")),
	}
	client := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(cfg.baseEndpoint)
		options.UsePathStyle = cfg.forcePathStyle
		if cfg.useChunkEncoding || usesHTTP(cfg.baseEndpoint) {
			options.APIOptions = append(options.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
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
		bucket: values[BucketKey], baseEndpoint: values[BaseEndpointKey],
		accessKeyID: values[AccessKeyIDKey], secretAccessKey: values[SecretAccessKeyKey],
		protocol: values[ProtocolKey],
	}
	for key, value := range map[string]string{
		BucketKey: cfg.bucket, BaseEndpointKey: cfg.baseEndpoint,
		AccessKeyIDKey: cfg.accessKeyID, SecretAccessKeyKey: cfg.secretAccessKey,
	} {
		if value == "" {
			return backendConfig{}, errors.New(key + " is required")
		}
	}
	baseEndpoint, err := url.ParseRequestURI(cfg.baseEndpoint)
	if err != nil || (baseEndpoint.Scheme != "http" && baseEndpoint.Scheme != "https") || baseEndpoint.Host == "" {
		return backendConfig{}, errors.New("base_endpoint must be an absolute HTTP(S) URL")
	}
	if cfg.protocol == "" {
		cfg.protocol = baseEndpoint.Scheme
	}
	if cfg.protocol != "http" && cfg.protocol != "https" {
		return backendConfig{}, errors.New("protocol must be http or https")
	}
	if cfg.forcePathStyle, err = readBool(values, ForcePathStyleKey); err != nil {
		return backendConfig{}, err
	}
	if cfg.useChunkEncoding, err = readBool(values, UseChunkEncodingKey); err != nil {
		return backendConfig{}, err
	}
	if cfg.createBucketIfNotExists, err = readBool(values, CreateBucketIfNotExistsKey); err != nil {
		return backendConfig{}, err
	}
	return cfg, nil
}

func readBool(values map[string]string, key string) (bool, error) {
	if values[key] == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(values[key])
	if err != nil {
		return false, errors.New(key + " must be a boolean")
	}
	return value, nil
}

func ensureBucket(ctx context.Context, client *awss3.Client, cfg backendConfig) error {
	if !cfg.createBucketIfNotExists {
		return nil
	}
	_, err := client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(cfg.bucket)})
	if err == nil {
		return nil
	}
	if !isNotFound(err) {
		return err
	}
	_, err = client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(cfg.bucket)})
	if !isBucketCreateConflict(err) {
		return err
	}
	_, err = client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(cfg.bucket)})
	return err
}

func fixedLengthBody(ctx context.Context, reader io.Reader) (io.Reader, *int64, func(), error) {
	if sized, ok := reader.(interface{ Len() int }); ok {
		size := int64(sized.Len())
		return reader, &size, func() {}, nil
	}
	if seeker, ok := reader.(io.Seeker); ok {
		current, currentErr := seeker.Seek(0, io.SeekCurrent)
		if currentErr == nil {
			end, endErr := seeker.Seek(0, io.SeekEnd)
			_, resetErr := seeker.Seek(current, io.SeekStart)
			if endErr == nil && resetErr == nil {
				size := end - current
				return reader, &size, func() {}, nil
			}
			if resetErr != nil {
				return nil, nil, func() {}, fmt.Errorf("restore upload reader position: %w", resetErr)
			}
		}
	}
	file, err := os.CreateTemp("", "sharp-store-s3-upload-*")
	if err != nil {
		return nil, nil, func() {}, err
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}
	size, err := io.Copy(file, &contextReader{ctx: ctx, reader: reader})
	if err != nil {
		cleanup()
		return nil, nil, func() {}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, func() {}, err
	}
	return file, &size, cleanup, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func usesHTTP(baseEndpoint string) bool {
	parsed, err := url.Parse(baseEndpoint)
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
