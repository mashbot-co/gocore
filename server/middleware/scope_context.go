package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore"
	"github.com/mashbot-co/gocore/db/connection"
)

// ScopeContext is the generic scope-context middleware: it lifts the
// configured scope ID (e.g. project_id, tenant_id) from gin context — set
// upstream by JWTAuth from the matching JWT claim, or by the stub-header
// path in dev — onto the request's GORM context via connection.WithCurrent*
// helpers. After this runs, mixin callbacks that scope queries by the
// chosen axis fire automatically.
//
// Lookup order:
//
//  1. gin context key matching gocore.ScopeIDClaim (set by JWTAuth in
//     release mode after validating the token).
//  2. Stub header named <StubHeaderPrefix><ScopeName-capitalised>-Id (e.g.
//     "X-Iro-Project-Id"), dev-mode shortcut so the playground works
//     without a real /auth/sync round-trip.
//
// Routes the looked-up ID to the correct connection-package setter based
// on gocore.ScopeName — "project" → WithCurrentProject, "tenant" →
// WithCurrentTenant. Other names fall through (no-op) since gocore's
// mixin set doesn't cover them yet.
func ScopeContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := scopeIDFromRequest(c)
		if id != uuid.Nil {
			c.Request = c.Request.WithContext(setScopeOnGORM(c.Request.Context(), id))
		}
		c.Next()
	}
}

func scopeIDFromRequest(c *gin.Context) uuid.UUID {
	claim := gocore.ScopeIDClaim()
	if claim == "" {
		return uuid.Nil
	}
	if v, ok := c.Get(claim); ok {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	if name := gocore.ScopeName(); name != "" {
		header := gocore.StubHeaderPrefix() + capitalizeFirst(name) + "-Id"
		if v := c.GetHeader(header); v != "" {
			if id, err := uuid.Parse(v); err == nil {
				return id
			}
		}
	}
	return uuid.Nil
}

// setScopeOnGORM routes the scope ID to the appropriate connection-package
// setter. Adding a new scope concept here is the smallest change required
// to teach gocore's middleware about it.
func setScopeOnGORM(ctx context.Context, id uuid.UUID) context.Context {
	switch gocore.ScopeName() {
	case "project":
		return connection.WithCurrentProject(ctx, id)
	case "tenant":
		return connection.WithCurrentTenant(ctx, id)
	}
	return ctx
}

// capitalizeFirst upper-cases the first rune of s, leaving the rest
// alone. Used to build header names from a lowercase ScopeName
// ("project" → "Project", yielding "X-Iro-Project-Id").
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
