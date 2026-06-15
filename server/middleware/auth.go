package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore"
	gocoreauth "github.com/mashbot-co/gocore/server/auth"
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

// OptionalAuth is the dev-mode GraphQL gate. It never rejects — anonymous
// requests pass so public fields and the playground work — but it resolves an
// identity two ways:
//
//   - Stub headers (X-<App>-Stub-User-Id): manual dev without a real session.
//   - A real Bearer JWT: e.g. a sibling app (Console/Platform) forwarding the
//     nutility_session cookie. We verify it and lift claims so dev-mode auth
//     matches production for genuine sessions. This is what lets the dashboards
//     authenticate against a locally-running API.
//
// A present-but-invalid token (stale/expired cookie) is left unauthenticated
// rather than aborting, so the caller simply bounces to re-login instead of
// erroring; the field directive still rejects protected fields.
func OptionalAuth(claimsFactory func() jwt.Claims, claimsHandler func(c *gin.Context, claims jwt.Claims)) gin.HandlerFunc {
	return func(c *gin.Context) {
		applyStubHeaders(c)

		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			raw := strings.TrimPrefix(h, "Bearer ")
			claims := claimsFactory()
			if _, err := gocoreauth.VerifyToken(raw, claims); err == nil && claimsHandler != nil {
				claimsHandler(c, claims)
			}
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
