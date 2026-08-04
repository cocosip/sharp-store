package s3compat

import (
	"context"
	"io"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/s3"
)

// ConfigTranslator converts one S3-compatible provider's public configuration
// into the connection settings consumed by the shared S3 transport.
type ConfigTranslator interface {
	Name() string
	ConfigOptions() []store.ConfigOption
	Translate(values map[string]string) (map[string]string, error)
}

type Backend struct {
	translator ConfigTranslator
	transport  *s3.Backend
}

func New(translator ConfigTranslator) *Backend {
	return &Backend{translator: translator, transport: s3.New()}
}

func (b *Backend) Name() string {
	return b.translator.Name()
}

func (b *Backend) ConfigOptions() []store.ConfigOption {
	return b.translator.ConfigOptions()
}

func (b *Backend) ValidateConfig(ctx context.Context, values map[string]string) error {
	translated, err := b.translator.Translate(values)
	if err != nil {
		return err
	}
	return b.transport.ValidateConfig(ctx, translated)
}

func (b *Backend) Save(ctx context.Context, request store.SaveRequest) (string, error) {
	fileRequest, err := b.translate(request.FileRequest)
	if err != nil {
		return "", err
	}
	request.FileRequest = fileRequest
	return b.transport.Save(ctx, request)
}

func (b *Backend) Delete(ctx context.Context, request store.FileRequest) (bool, error) {
	fileRequest, err := b.translate(request)
	if err != nil {
		return false, err
	}
	return b.transport.Delete(ctx, fileRequest)
}

func (b *Backend) Exists(ctx context.Context, request store.FileRequest) (bool, error) {
	fileRequest, err := b.translate(request)
	if err != nil {
		return false, err
	}
	return b.transport.Exists(ctx, fileRequest)
}

func (b *Backend) Download(ctx context.Context, request store.DownloadRequest) (bool, error) {
	fileRequest, err := b.translate(request.FileRequest)
	if err != nil {
		return false, err
	}
	request.FileRequest = fileRequest
	return b.transport.Download(ctx, request)
}

func (b *Backend) GetOrNil(ctx context.Context, request store.FileRequest) (io.ReadCloser, error) {
	fileRequest, err := b.translate(request)
	if err != nil {
		return nil, err
	}
	return b.transport.GetOrNil(ctx, fileRequest)
}

func (b *Backend) AccessURL(ctx context.Context, request store.AccessURLRequest) (string, error) {
	fileRequest, err := b.translate(request.FileRequest)
	if err != nil {
		return "", err
	}
	request.FileRequest = fileRequest
	return b.transport.AccessURL(ctx, request)
}

func (b *Backend) translate(request store.FileRequest) (store.FileRequest, error) {
	values, err := b.translator.Translate(request.Config.Values)
	if err != nil {
		return store.FileRequest{}, err
	}
	request.Config = request.Config.Clone()
	request.Config.Values = values
	return request, nil
}
