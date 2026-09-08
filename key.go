package store

import (
	"context"
	"fmt"
	"strings"
)

// KeyBuilder implementations must be safe for concurrent use by multiple
// goroutines when supplied to a Factory.
type KeyBuilder interface {
	Build(ctx context.Context, request FileRequest) (string, error)
}

type defaultKeyBuilder struct{}

func (defaultKeyBuilder) Build(_ context.Context, request FileRequest) (string, error) {
	fileID := strings.Trim(request.FileID, "/")
	if strings.Trim(fileID, "\\") == "" {
		return "", fmt.Errorf("invalid file ID %q", request.FileID)
	}
	for _, segment := range strings.FieldsFunc(fileID, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid file ID %q", request.FileID)
		}
	}
	segments := make([]string, 0, 2)
	if request.Config.TenantMode == TenantScoped && request.Tenant != nil {
		identifier := request.Tenant.TenantCode()
		if identifier == "" {
			identifier = request.Tenant.TenantID()
		}
		if identifier != "" {
			if identifier == "." || identifier == ".." || strings.ContainsAny(identifier, "/\\") {
				return "", fmt.Errorf("invalid tenant key segment %q", identifier)
			}
			segments = append(segments, identifier)
		}
	}
	segments = append(segments, fileID)
	return strings.Join(segments, "/"), nil
}
