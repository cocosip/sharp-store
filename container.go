package store

import (
	"context"
	"io"
	"time"
)

type AccessURLOptions struct {
	ExpiresAt   *time.Time
	CheckExists bool
}

type container struct {
	key     ContainerKey
	config  ContainerConfig
	tenant  TenantContext
	backend Backend
	names   NamingService
	keys    KeyBuilder
}

func (c *container) Configuration() ContainerConfig {
	return c.config.Clone()
}

func (c *container) Save(
	ctx context.Context,
	fileID string,
	body io.Reader,
	overwrite bool,
) (string, error) {
	request, err := c.request(ctx, fileID)
	if err != nil {
		return "", err
	}
	return c.backend.Save(ctx, SaveRequest{
		FileRequest: request,
		Body:        body,
		Overwrite:   overwrite,
	})
}

func (c *container) Delete(ctx context.Context, fileID string) (bool, error) {
	request, err := c.request(ctx, fileID)
	if err != nil {
		return false, err
	}
	return c.backend.Delete(ctx, request)
}

func (c *container) Exists(ctx context.Context, fileID string) (bool, error) {
	request, err := c.request(ctx, fileID)
	if err != nil {
		return false, err
	}
	return c.backend.Exists(ctx, request)
}

func (c *container) Download(ctx context.Context, fileID, destination string) (bool, error) {
	request, err := c.request(ctx, fileID)
	if err != nil {
		return false, err
	}
	return c.backend.Download(ctx, DownloadRequest{FileRequest: request, Destination: destination})
}

func (c *container) Get(ctx context.Context, fileID string) (io.ReadCloser, error) {
	reader, err := c.GetOrNil(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, ErrFileNotFound
	}
	return reader, nil
}

func (c *container) GetOrNil(ctx context.Context, fileID string) (io.ReadCloser, error) {
	request, err := c.request(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return c.backend.GetOrNil(ctx, request)
}

func (c *container) AccessURL(ctx context.Context, fileID string, options AccessURLOptions) (string, error) {
	request, err := c.request(ctx, fileID)
	if err != nil {
		return "", err
	}
	return c.backend.AccessURL(ctx, AccessURLRequest{
		FileRequest: request,
		ExpiresAt:   options.ExpiresAt,
		CheckExists: options.CheckExists,
	})
}

func (c *container) request(ctx context.Context, fileID string) (FileRequest, error) {
	normalized, err := c.names.Normalize(ctx, c.config, c.key, fileID)
	if err != nil {
		return FileRequest{}, err
	}
	request := FileRequest{
		Container: normalized.Container,
		Config:    c.config.Clone(),
		Tenant:    c.tenant,
		FileID:    normalized.FileID,
	}
	key, err := c.keys.Build(ctx, request)
	if err != nil {
		return FileRequest{}, err
	}
	request.Key = key
	return request, nil
}
