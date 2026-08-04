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

func (s Scope) clone() Scope {
	copy := s
	if s.Values != nil {
		copy.Values = make(map[string]string, len(s.Values))
		for key, value := range s.Values {
			copy.Values[key] = value
		}
	}
	return copy
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
