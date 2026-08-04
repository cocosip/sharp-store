package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	store "github.com/cocosip/sharp-store"
)

const (
	Name       = "filesystem"
	RootKey    = "root"
	BaseURLKey = "base_url"
)

type Backend struct{}

func New() *Backend {
	return &Backend{}
}

func (*Backend) Name() string {
	return Name
}

func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{
		{
			Name:        RootKey,
			Type:        "string",
			Required:    true,
			Example:     "D:/sharp-store",
			Description: "Root directory for stored objects.",
		},
		{
			Name:        BaseURLKey,
			Type:        "string",
			Example:     "https://files.example.test",
			Description: "Optional public base URL for object access URLs.",
		},
	}
}

func (*Backend) ValidateConfig(ctx context.Context, values map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if values[RootKey] == "" {
		return errors.New("root is required")
	}
	return nil
}

func (b *Backend) Save(ctx context.Context, request store.SaveRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if request.Body == nil {
		return "", errors.New("file body is required")
	}
	path, err := b.path(request.FileRequest)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	flags := os.O_WRONLY | os.O_CREATE
	if request.Overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", store.ErrFileExists, request.FileID)
		}
		return "", err
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, request.Body); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return request.FileID, nil
}

func (b *Backend) Delete(ctx context.Context, request store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := b.path(request)
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (b *Backend) Exists(ctx context.Context, request store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := b.path(request)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (b *Backend) Download(ctx context.Context, request store.DownloadRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	reader, err := b.GetOrNil(ctx, request.FileRequest)
	if err != nil {
		return false, err
	}
	if reader == nil {
		return false, nil
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := b.path(request)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return file, nil
}

func (b *Backend) AccessURL(ctx context.Context, request store.AccessURLRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if request.CheckExists {
		exists, err := b.Exists(ctx, request.FileRequest)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", store.ErrFileNotFound
		}
	}
	baseURL := request.Config.Values[BaseURLKey]
	if baseURL == "" {
		return "", store.ErrUnsupported
	}
	return url.JoinPath(baseURL, request.Key)
}

func (b *Backend) path(request store.FileRequest) (string, error) {
	root := request.Config.Values[RootKey]
	if root == "" {
		return "", fmt.Errorf("%s configuration is required", RootKey)
	}
	key := filepath.Clean(filepath.FromSlash(request.Key))
	if key == "." || key == ".." || filepath.IsAbs(key) || strings.HasPrefix(key, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid object key %q", request.Key)
	}
	path := filepath.Join(root, key)
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("object key escapes configured root: %q", request.Key)
	}
	return path, nil
}
