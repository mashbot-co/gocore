package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore"
)

// Auth validates the request's identity. Real JWT verification (provider JWKS)
// will be wired in once gocore/auth gains a verifier — this stub keeps the
// structure intact:
//
//   - In production (GIN_MODE=release): every request is rejected with 401
//     until verification lands. That's intentional — we'd rather fail loudly
//     than accidentally ship an open endpoint.
//   - In dev / test: user_id is read from a stub header (default
//     "X-Stub-User-Id"). The consumer's stub-header prefix is configurable
//     via gocore.Init — e.g. setting StubHeaderPrefix to "X-Iro-Stub-"
//     yields "X-Iro-Stub-User-Id". Consumers that need additional scope IDs
//     (tenant, project, workspace, etc) layer their own middleware on top —
//     gocore stays scope-agnostic.
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

// applyStubHeaders reads the configured stub user header and populates
// the Gin context. Shared between Auth (dev mode) and OptionalAuth.
func applyStubHeaders(c *gin.Context) {
	if v := c.GetHeader(gocore.StubHeaderPrefix() + "User-Id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			c.Set("user_id", id)
		}
	}
}
