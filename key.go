package store

import (
	"context"
	"strings"
)

// KeyBuilder implementations must be safe for concurrent use by multiple
// goroutines when supplied to a Factory.
type KeyBuilder interface {
	Build(ctx context.Context, request FileRequest) (string, error)
}

type defaultKeyBuilder struct{}

func (defaultKeyBuilder) Build(_ context.Context, request FileRequest) (string, error) {
	segments := make([]string, 0, 2)
	if request.Config.TenantMode == TenantScoped && request.Tenant != nil {
		identifier := request.Tenant.TenantCode()
		if identifier == "" {
			identifier = request.Tenant.TenantID()
		}
		if identifier != "" {
			segments = append(segments, identifier)
		}
	}
	segments = append(segments, strings.Trim(request.FileID, "/"))
	return strings.Join(segments, "/"), nil
}
