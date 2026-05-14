package connection

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	currentUserKey    contextKey = "current_user_id"
	currentTenantKey  contextKey = "current_tenant_id"
	currentProjectKey contextKey = "current_project_id"
	isAdminKey        contextKey = "is_admin"
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

// WithCurrentProject returns a new context with the current project ID set.
// Set this from the auth middleware (or a resolver) after resolving the request's project.
func WithCurrentProject(ctx context.Context, projectID uuid.UUID) context.Context {
	return context.WithValue(ctx, currentProjectKey, projectID)
}

// CurrentProject extracts the current project ID from context. Returns uuid.Nil if not set.
// Used by ProjectMixin callbacks to populate project_id on create and inject
// WHERE project_id = ? on query / update / delete.
func CurrentProject(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(currentProjectKey).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

// WithoutProjectScope returns a context with project scoping removed.
// Queries using this context will not be filtered by project_id.
// Use sparingly — typically for cross-project admin operations or
// for the membership-resolution step that runs before project scope is known.
func WithoutProjectScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, currentProjectKey, uuid.Nil)
}

// WithIsAdmin returns a new context with the system-admin flag set. Auth
// middleware calls this after lifting the corresponding JWT claim;
// authorization-layer code (e.g. authz directives / role resolvers) can
// short-circuit role lookups when this is true.
func WithIsAdmin(ctx context.Context, isAdmin bool) context.Context {
	if !isAdmin {
		return ctx
	}
	return context.WithValue(ctx, isAdminKey, true)
}

// IsAdmin reports whether the caller is a system-wide admin. False when
// unset or explicitly false. Note: this is intentionally a single binary
// axis — finer permissions live in the scope catalog / role bundles.
func IsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(isAdminKey).(bool)
	return v
}

// WithoutTenantScope returns a context with tenant scoping removed.
// Queries using this context will not be filtered by tenant_id.
// The user ID is preserved for authentication checks. Use sparingly —
// typically only for cross-tenant admin operations.
func WithoutTenantScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, currentTenantKey, uuid.Nil)
}
