package connection

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	currentUserKey   contextKey = "current_user_id"
	currentTenantKey contextKey = "current_tenant_id"
)

// WithCurrentUser returns a new context with the current user ID set.
// Set this from the auth middleware after verifying the request's identity.
func WithCurrentUser(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, currentUserKey, userID)
}

// CurrentUser extracts the current user ID from context. Returns uuid.Nil if not set.
// Used by TrackedMixin callbacks to populate created_by / updated_by.
func CurrentUser(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(currentUserKey).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

// WithCurrentTenant returns a new context with the current tenant ID set.
// Set this from the auth middleware after resolving the request's tenant.
func WithCurrentTenant(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, currentTenantKey, tenantID)
}

// CurrentTenant extracts the current tenant ID from context. Returns uuid.Nil if not set.
// Used by TenantMixin callbacks to populate tenant_id on create and inject
// WHERE tenant_id = ? on query / update / delete.
func CurrentTenant(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(currentTenantKey).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

// WithoutTenantScope returns a context with tenant scoping removed.
// Queries using this context will not be filtered by tenant_id.
// The user ID is preserved for authentication checks. Use sparingly —
// typically only for cross-tenant admin operations.
func WithoutTenantScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, currentTenantKey, uuid.Nil)
}
