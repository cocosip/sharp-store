package obs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	store "github.com/cocosip/sharp-store"
	obsdk "github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
)

const (
	Name               = "obs"
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
	client, bucket, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return "", err
	}
	_, err = client.PutObject(&obsdk.PutObjectInput{PutObjectBasicInput: obsdk.PutObjectBasicInput{ObjectOperationInput: obsdk.ObjectOperationInput{Bucket: bucket, Key: r.Key}}, Body: r.Body}, obsdk.WithCustomHeader("x-obs-forbid-overwrite", strconv.FormatBool(!r.Overwrite)))
	var serviceError obsdk.ObsError
	if !r.Overwrite && errors.As(err, &serviceError) && serviceError.Code == "ObjectAlreadyExists" {
		return "", store.ErrFileExists
	}
	return r.FileID, err
}
func (b *Backend) Delete(ctx context.Context, r store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	client, bucket, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = client.DeleteObject(&obsdk.DeleteObjectInput{Bucket: bucket, Key: r.Key})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}
func (b *Backend) Exists(ctx context.Context, r store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	client, bucket, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = client.HeadObject(&obsdk.HeadObjectInput{Bucket: bucket, Key: r.Key})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}
func (b *Backend) Download(ctx context.Context, r store.DownloadRequest) (bool, error) {
	reader, err := b.GetOrNil(ctx, r.FileRequest)
	if err != nil || reader == nil {
		return false, err
	}
	defer func() { _ = reader.Close() }()
	if err = os.MkdirAll(filepath.Dir(r.Destination), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(r.Destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, reader)
	return err == nil, err
}
func (b *Backend) GetOrNil(ctx context.Context, r store.FileRequest) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, bucket, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return nil, err
	}
	out, err := client.GetObject(&obsdk.GetObjectInput{GetObjectMetadataInput: obsdk.GetObjectMetadataInput{Bucket: bucket, Key: r.Key}})
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return out.Body, nil
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
	client, bucket, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return "", err
	}
	expires := 3600
	if r.ExpiresAt != nil {
		d := time.Until(*r.ExpiresAt)
		if d <= 0 {
			return "", errors.New("access URL expiry is in the past")
		}
		expires = int(d.Seconds())
	}
	out, err := client.CreateSignedUrl(&obsdk.CreateSignedUrlInput{Method: obsdk.HttpMethodGet, Bucket: bucket, Key: r.Key, Expires: expires})
	if err != nil {
		return "", err
	}
	return out.SignedUrl, nil
}
func (b *Backend) client(ctx context.Context, v map[string]string) (*obsdk.ObsClient, string, error) {
	if err := b.ValidateConfig(ctx, v); err != nil {
		return nil, "", err
	}
	key := v[EndpointKey] + "\x00" + v[AccessKeyIDKey] + "\x00" + v[AccessKeySecretKey]
	if x, ok := b.clients.Load(key); ok {
		return x.(*obsdk.ObsClient), v[BucketKey], nil
	}
	c, err := obsdk.New(v[AccessKeyIDKey], v[AccessKeySecretKey], v[EndpointKey])
	if err != nil {
		return nil, "", err
	}
	x, loaded := b.clients.LoadOrStore(key, c)
	if loaded {
		c = x.(*obsdk.ObsClient)
	}
	return c, v[BucketKey], nil
}
func isNotFound(err error) bool {
	var e obsdk.ObsError
	return errors.As(err, &e) && (e.Code == "NoSuchKey" || e.Code == "NoSuchObject" || e.Status == "404")
}
