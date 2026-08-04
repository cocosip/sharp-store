package store

import (
	"context"
	"strings"
)

type KeyBuilder interface {
	Build(ctx context.Context, request FileRequest) (string, error)
}

type defaultKeyBuilder struct{}

func (defaultKeyBuilder) Build(_ context.Context, request FileRequest) (string, error) {
	segments := make([]string, 0, 3)
	if request.Scope.Prefix != "" {
		segments = append(segments, strings.Trim(request.Scope.Prefix, "/"))
	}
	if tenant := request.Scope.Tenant; tenant.ID != "" {
		identifier := tenant.Code
		if identifier == "" {
			identifier = tenant.ID
		}
		segments = append(segments, identifier)
	}
	segments = append(segments, strings.Trim(request.FileID, "/"))
	return strings.Join(segments, "/"), nil
}
