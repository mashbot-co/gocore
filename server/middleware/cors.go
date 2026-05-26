// Package middleware provides Gin middleware shared across all gocore-based
// APIs: CORS, request-ID propagation, tenant context wiring, and auth.
package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS adds Cross-Origin Resource Sharing headers.
//
// When CORS_ALLOWED_ORIGINS is set (comma-separated list of exact origins),
// credentialed cross-origin requests are honoured for those origins: the
// Origin header is reflected back and Access-Control-Allow-Credentials is
// emitted, so the browser will accept Set-Cookie on the response and send
// the cookie on subsequent requests.
//
// Anything outside the allowlist (and any request with no Origin header)
// falls back to `Access-Control-Allow-Origin: *` — fine for public reads
// like /health and the GraphQL playground, but the browser will block any
// credentialed fetch against this response. That asymmetry is deliberate.
func CORS() gin.HandlerFunc {
	allowed := parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func parseAllowedOrigins(s string) map[string]bool {
	if s == "" {
		return nil
	}
	out := make(map[string]bool)
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			out[o] = true
		}
	}
	return out
}
