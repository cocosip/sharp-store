package aliyun

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	store "github.com/cocosip/sharp-store"
)

const (
	Name               = "aliyun"
	EndpointKey        = "endpoint"
	BucketKey          = "bucket"
	AccessKeyIDKey     = "access_key_id"
	AccessKeySecretKey = "access_key_secret"
)

type Backend struct{ clients sync.Map }

func New() store.Backend      { return &Backend{} }
func (*Backend) Name() string { return Name }
func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{{Name: EndpointKey, Type: "string", Required: true}, {Name: BucketKey, Type: "string", Required: true}, {Name: AccessKeyIDKey, Type: "string", Required: true, Sensitive: true}, {Name: AccessKeySecretKey, Type: "string", Required: true, Sensitive: true}}
}
func (*Backend) ValidateConfig(ctx context.Context, v map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, k := range []string{EndpointKey, BucketKey, AccessKeyIDKey, AccessKeySecretKey} {
		if v[k] == "" {
			return errors.New(k + " is required")
		}
	}
	return nil
}
func (b *Backend) Save(ctx context.Context, r store.SaveRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if r.Body == nil {
		return "", errors.New("file body is required")
	}
	bucket, err := b.bucket(ctx, r.Config.Values)
	if err != nil {
		return "", err
	}
	err = bucket.PutObject(r.Key, r.Body, oss.ForbidOverWrite(!r.Overwrite), oss.WithContext(ctx))
	var serviceError oss.ServiceError
	if !r.Overwrite && errors.As(err, &serviceError) && serviceError.Code == "FileAlreadyExists" {
		return "", store.ErrFileExists
	}
	return r.FileID, err
}
func (b *Backend) Delete(ctx context.Context, r store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	bucket, err := b.bucket(ctx, r.Config.Values)
	if err != nil {
		return false, err
	}
	exists, err := bucket.IsObjectExist(r.Key)
	if err != nil || !exists {
		return false, err
	}
	err = bucket.DeleteObject(r.Key)
	return err == nil, err
}
func (b *Backend) Exists(ctx context.Context, r store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	bucket, err := b.bucket(ctx, r.Config.Values)
	if err != nil {
		return false, err
	}
	return bucket.IsObjectExist(r.Key)
}
func (b *Backend) Download(ctx context.Context, r store.DownloadRequest) (bool, error) {
	bucket, err := b.bucket(ctx, r.Config.Values)
	if err != nil {
		return false, err
	}
	if err = os.MkdirAll(filepath.Dir(r.Destination), 0o755); err != nil {
		return false, err
	}
	err = bucket.GetObjectToFile(r.Key, r.Destination)
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}
func (b *Backend) GetOrNil(ctx context.Context, r store.FileRequest) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bucket, err := b.bucket(ctx, r.Config.Values)
	if err != nil {
		return nil, err
	}
	reader, err := bucket.GetObject(r.Key)
	if isNotFound(err) {
		return nil, nil
	}
	return reader, err
}
func (b *Backend) AccessURL(ctx context.Context, r store.AccessURLRequest) (string, error) {
	if r.CheckExists {
		ok, err := b.Exists(ctx, r.FileRequest)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", store.ErrFileNotFound
		}
	}
	bucket, err := b.bucket(ctx, r.Config.Values)
	if err != nil {
		return "", err
	}
	seconds := int64(3600)
	if r.ExpiresAt != nil {
		d := time.Until(*r.ExpiresAt)
		if d <= 0 {
			return "", errors.New("access URL expiry is in the past")
		}
		seconds = int64(d.Seconds())
	}
	return bucket.SignURL(r.Key, oss.HTTPGet, seconds)
}
func (b *Backend) bucket(ctx context.Context, v map[string]string) (*oss.Bucket, error) {
	if err := b.ValidateConfig(ctx, v); err != nil {
		return nil, err
	}
	key := v[EndpointKey] + "\x00" + v[AccessKeyIDKey] + "\x00" + v[AccessKeySecretKey]
	var client *oss.Client
	if x, ok := b.clients.Load(key); ok {
		client = x.(*oss.Client)
	} else {
		c, err := oss.New(v[EndpointKey], v[AccessKeyIDKey], v[AccessKeySecretKey])
		if err != nil {
			return nil, err
		}
		x, loaded := b.clients.LoadOrStore(key, c)
		if loaded {
			client = x.(*oss.Client)
		} else {
			client = c
		}
	}
	return client.Bucket(v[BucketKey])
}
func isNotFound(err error) bool {
	var e oss.ServiceError
	return errors.As(err, &e) && (e.Code == "NoSuchKey" || e.Code == "NoSuchObject" || e.StatusCode == 404)
}
