// Package tenant is a sample service for testing the gqlgen tool's
// annotation parsing. The functions below are not meant to run — they
// just carry the annotations the parser looks for.
package tenant

import (
	"context"

	"github.com/google/uuid"
)

// SwitchTenantResult is a service response type — should be discovered as
// a ServiceTypeDef when ParseService walks this directory.
type SwitchTenantResult struct {
	Success  bool      `json:"success"`
	TenantID uuid.UUID `json:"tenant_id"`
	Message  string    `json:"message"`
}

// CurrentTenant returns the tenant the caller currently belongs to.
//
// graphql:query:currentTenant() Tenant
func CurrentTenant(ctx context.Context) (interface{}, error) {
	return nil, nil
}

// MyTenants returns every tenant the user is a member of.
//
// graphql:query:myTenants() []Tenant @public
func MyTenants(ctx context.Context) ([]interface{}, error) {
	return nil, nil
}

// SwitchTenant changes the caller's active tenant.
//
// graphql:mutation:switchTenant(tenantId uuid.UUID) SwitchTenantResult
func SwitchTenant(ctx context.Context, tenantID uuid.UUID) (interface{}, error) {
	return nil, nil
}
