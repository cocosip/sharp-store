package azure

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	store "github.com/cocosip/sharp-store"
)

const (
	Name                = "azure"
	ConnectionStringKey = "connection_string"
	ContainerKey        = "container"
)

type Backend struct{ clients sync.Map }

func New() store.Backend      { return &Backend{} }
func (*Backend) Name() string { return Name }
func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{{Name: ConnectionStringKey, Type: "string", Required: true, Sensitive: true, Description: "Azure Storage connection string."}, {Name: ContainerKey, Type: "string", Required: true, Description: "Azure Blob container name."}}
}
func (*Backend) ValidateConfig(ctx context.Context, values map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if values[ConnectionStringKey] == "" {
		return errors.New("connection_string is required")
	}
	if values[ContainerKey] == "" {
		return errors.New("container is required")
	}
	return nil
}
func (b *Backend) Save(ctx context.Context, r store.SaveRequest) (string, error) {
	if r.Body == nil {
		return "", errors.New("file body is required")
	}
	c, container, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return "", err
	}
	options := &azblob.UploadBufferOptions{}
	if !r.Overwrite {
		etag := azcore.ETag("*")
		options.AccessConditions = &blob.AccessConditions{ModifiedAccessConditions: &blob.ModifiedAccessConditions{IfNoneMatch: &etag}}
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	_, err = c.UploadBuffer(ctx, container, r.Key, data, options)
	if err != nil {
		var responseError *azcore.ResponseError
		if !r.Overwrite && errors.As(err, &responseError) && (responseError.ErrorCode == "ConditionNotMet" || responseError.ErrorCode == "BlobAlreadyExists") {
			return "", store.ErrFileExists
		}
		return "", err
	}
	return r.FileID, nil
}
func (b *Backend) Delete(ctx context.Context, r store.FileRequest) (bool, error) {
	c, container, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = c.DeleteBlob(ctx, container, r.Key, nil)
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}
func (b *Backend) Exists(ctx context.Context, r store.FileRequest) (bool, error) {
	c, container, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return false, err
	}
	_, err = c.ServiceClient().NewContainerClient(container).NewBlobClient(r.Key).GetProperties(ctx, nil)
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
	if err := os.MkdirAll(filepath.Dir(r.Destination), 0o755); err != nil {
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
	c, container, err := b.client(ctx, r.Config.Values)
	if err != nil {
		return nil, err
	}
	out, err := c.DownloadStream(ctx, container, r.Key, nil)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return out.NewRetryReader(ctx, nil), nil
}
func (*Backend) AccessURL(context.Context, store.AccessURLRequest) (string, error) {
	return "", store.ErrUnsupported
}
func (b *Backend) client(ctx context.Context, values map[string]string) (*azblob.Client, string, error) {
	if err := b.ValidateConfig(ctx, values); err != nil {
		return nil, "", err
	}
	key := values[ConnectionStringKey]
	if cached, ok := b.clients.Load(key); ok {
		return cached.(*azblob.Client), values[ContainerKey], nil
	}
	client, err := azblob.NewClientFromConnectionString(key, nil)
	if err != nil {
		return nil, "", err
	}
	actual, loaded := b.clients.LoadOrStore(key, client)
	if loaded {
		client = actual.(*azblob.Client)
	}
	return client, values[ContainerKey], nil
}
func isNotFound(err error) bool {
	var responseError *azcore.ResponseError
	return errors.As(err, &responseError) && responseError.StatusCode == 404
}
