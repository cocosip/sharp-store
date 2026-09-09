package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	store "github.com/cocosip/sharp-store"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	Name                       = "minio"
	BucketKey                  = "bucket"
	EndpointKey                = "endpoint"
	AccessKeyKey               = "access_key"
	SecretKeyKey               = "secret_key"
	RegionKey                  = "region"
	WithSSLKey                 = "with_ssl"
	CreateBucketIfNotExistsKey = "create_bucket_if_not_exists"
)

const defaultAccessURLExpiry = 15 * time.Minute
const defaultRegion = "us-east-1"

type backendConfig struct {
	bucket                  string
	endpoint                string
	accessKey               string
	secretKey               string
	region                  string
	withSSL                 bool
	createBucketIfNotExists bool
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
		{Name: BucketKey, Type: "string", Required: true, Example: "dicom-archive", Description: "MinIO bucket name."},
		{Name: EndpointKey, Type: "string", Required: true, Example: "minio.example.test:9000", Description: "MinIO host or IP with an optional port, without a URL scheme."},
		{Name: AccessKeyKey, Type: "string", Required: true, Sensitive: true, Description: "MinIO access key."},
		{Name: SecretKeyKey, Type: "string", Required: true, Sensitive: true, Description: "MinIO secret key."},
		{Name: RegionKey, Type: "string", Example: defaultRegion, Description: "MinIO region used for request signing. Defaults to us-east-1."},
		{Name: WithSSLKey, Type: "bool", Example: "true", Description: "Use HTTPS for the configured endpoint."},
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
	size, err := uploadSize(request.Body)
	if err != nil {
		return "", err
	}
	options := miniosdk.PutObjectOptions{}
	if !request.Overwrite {
		options.SetMatchETagExcept("*")
	}
	_, err = client.PutObject(ctx, cfg.bucket, request.Key, request.Body, size, options)
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
	err = client.RemoveObject(ctx, cfg.bucket, request.Key, miniosdk.RemoveObjectOptions{})
	return err == nil, err
}

func (b *Backend) Exists(ctx context.Context, request store.FileRequest) (bool, error) {
	client, cfg, err := b.client(ctx, request.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = client.StatObject(ctx, cfg.bucket, request.Key, miniosdk.StatObjectOptions{})
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
	if _, err = client.StatObject(ctx, cfg.bucket, request.Key, miniosdk.StatObjectOptions{}); err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return client.GetObject(ctx, cfg.bucket, request.Key, miniosdk.GetObjectOptions{})
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
	presigned, err := client.PresignedGetObject(ctx, cfg.bucket, request.Key, expiresIn, nil)
	if err != nil {
		return "", err
	}
	return presigned.String(), nil
}

func (b *Backend) client(ctx context.Context, values map[string]string) (*miniosdk.Client, backendConfig, error) {
	cfg, err := readConfig(values)
	if err != nil {
		return nil, backendConfig{}, err
	}
	if client, ok := b.clients.Load(cfg); ok {
		return client.(*miniosdk.Client), cfg, nil
	}
	client, err := miniosdk.New(cfg.endpoint, &miniosdk.Options{
		Creds:        credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, ""),
		Secure:       cfg.withSSL,
		Region:       cfg.region,
		BucketLookup: miniosdk.BucketLookupPath,
	})
	if err != nil {
		return nil, backendConfig{}, err
	}
	actual, loaded := b.clients.LoadOrStore(cfg, client)
	if loaded {
		client = actual.(*miniosdk.Client)
	}
	return client, cfg, nil
}

func uploadSize(reader io.Reader) (int64, error) {
	if sized, ok := reader.(interface{ Len() int }); ok {
		return int64(sized.Len()), nil
	}
	if seeker, ok := reader.(io.Seeker); ok {
		current, currentErr := seeker.Seek(0, io.SeekCurrent)
		if currentErr == nil {
			end, endErr := seeker.Seek(0, io.SeekEnd)
			_, resetErr := seeker.Seek(current, io.SeekStart)
			if resetErr != nil {
				return 0, fmt.Errorf("restore upload reader position: %w", resetErr)
			}
			if endErr == nil {
				return end - current, nil
			}
		}
	}
	return -1, nil
}

func readConfig(values map[string]string) (backendConfig, error) {
	cfg := backendConfig{
		bucket:    values[BucketKey],
		endpoint:  values[EndpointKey],
		accessKey: values[AccessKeyKey],
		secretKey: values[SecretKeyKey],
		region:    values[RegionKey],
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
	if cfg.region == "" {
		cfg.region = defaultRegion
	}
	if value := values[WithSSLKey]; value != "" {
		withSSL, err := strconv.ParseBool(value)
		if err != nil {
			return backendConfig{}, errors.New("with_ssl must be a boolean")
		}
		cfg.withSSL = withSSL
	}
	if value := values[CreateBucketIfNotExistsKey]; value != "" {
		create, err := strconv.ParseBool(value)
		if err != nil {
			return backendConfig{}, errors.New("create_bucket_if_not_exists must be a boolean")
		}
		cfg.createBucketIfNotExists = create
	}
	return cfg, nil
}

func ensureBucket(ctx context.Context, client *miniosdk.Client, cfg backendConfig) error {
	if !cfg.createBucketIfNotExists {
		return nil
	}
	exists, err := client.BucketExists(ctx, cfg.bucket)
	if err != nil || exists {
		return err
	}
	err = client.MakeBucket(ctx, cfg.bucket, miniosdk.MakeBucketOptions{})
	if !isBucketCreateConflict(err) {
		return err
	}
	exists, headErr := client.BucketExists(ctx, cfg.bucket)
	if headErr != nil {
		return headErr
	}
	if !exists {
		return err
	}
	return nil
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
	response := miniosdk.ToErrorResponse(err)
	return response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.Code == "NotFound"
}

func isConflict(err error) bool {
	response := miniosdk.ToErrorResponse(err)
	return response.Code == "PreconditionFailed" || response.Code == "ConditionalRequestConflict" || response.Code == "FileAlreadyExists" || response.Code == "ObjectAlreadyExists"
}

func isBucketCreateConflict(err error) bool {
	code := miniosdk.ToErrorResponse(err).Code
	return code == "BucketAlreadyOwnedByYou" || code == "BucketAlreadyExists"
}
