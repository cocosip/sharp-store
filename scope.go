package store

import "context"

type Tenant struct {
	ID   string
	Name string
	Code string
}

type Scope struct {
	Tenant Tenant
	Prefix string
	Values map[string]string
}

type ScopeResolver interface {
	Resolve(ctx context.Context) (Scope, error)
}

type emptyScopeResolver struct{}

func (emptyScopeResolver) Resolve(context.Context) (Scope, error) {
	return Scope{}, nil
}

func (s Scope) withoutTenant() Scope {
	s.Tenant = Tenant{}
	return s
}
