package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Auth validates the request's identity. Real JWT verification (provider JWKS)
// will be wired in once gocore/auth gains a verifier — this stub keeps the
// structure intact:
//
//   - In production (GIN_MODE=release): every request is rejected with 401
//     until verification lands. That's intentional — we'd rather fail loudly
//     than accidentally ship an open endpoint.
//   - In dev / test: tenant_id and user_id are read from X-Stub-Tenant-Id and
//     X-Stub-User-Id headers (if present). This lets us exercise mixin-driven
//     tenant scoping locally without a real auth flow.
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("GIN_MODE") == "release" {
			// TODO: Replace with gocore/auth JWKS verification.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "auth not yet implemented — provider integration pending",
			})
			return
		}

		applyStubHeaders(c)
		c.Next()
	}
}

// OptionalAuth is identical to Auth in dev mode and a no-op in production.
// Used in front of the GraphQL playground so introspection works without a
// token. Resolvers are still responsible for checking auth on data access.
func OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		applyStubHeaders(c)

		if h := c.GetHeader("Authorization"); h != "" && strings.HasPrefix(h, "Bearer ") {
			// TODO: when verification is wired up, parse the bearer token
			// here and populate tenant_id / user_id from the claims.
		}

		c.Next()
	}
}

// applyStubHeaders reads X-Stub-Tenant-Id / X-Stub-User-Id and populates the
// Gin context. Shared between Auth (dev mode) and OptionalAuth.
func applyStubHeaders(c *gin.Context) {
	if v := c.GetHeader("X-Stub-Tenant-Id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			c.Set("tenant_id", id)
		}
	}
	if v := c.GetHeader("X-Stub-User-Id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			c.Set("user_id", id)
		}
	}
}
