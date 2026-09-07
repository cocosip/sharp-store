package store

// TenantContext is the tenant information consumed by sharp-store. Tenant
// discovery and lifecycle remain the application's responsibility.
type TenantContext interface {
	TenantID() string
	TenantCode() string
	TenantName() string
}

// DefaultTenantContext is the built-in TenantContext implementation.
type DefaultTenantContext struct {
	ID   string
	Code string
	Name string
}

func (t DefaultTenantContext) TenantID() string   { return t.ID }
func (t DefaultTenantContext) TenantCode() string { return t.Code }
func (t DefaultTenantContext) TenantName() string { return t.Name }

// NoTenant returns the tenant context used for host-level storage.
func NoTenant() TenantContext { return DefaultTenantContext{} }

type tenantContextSnapshot struct {
	id   string
	code string
	name string
}

func (t tenantContextSnapshot) TenantID() string   { return t.id }
func (t tenantContextSnapshot) TenantCode() string { return t.code }
func (t tenantContextSnapshot) TenantName() string { return t.name }

func snapshotTenant(tenant TenantContext) TenantContext {
	if tenant == nil {
		return tenantContextSnapshot{}
	}
	return tenantContextSnapshot{
		id:   tenant.TenantID(),
		code: tenant.TenantCode(),
		name: tenant.TenantName(),
	}
}
