package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	gocoreauth "github.com/mashbot-co/gocore/server/auth"
)

// JWTAuth is the generic production auth middleware. It validates the
// inbound `Authorization: Bearer <jwt>` against the gocore-issued keypair
// (configured via gocoreauth.Init) and invokes the supplied claimsHandler
// callback after a successful parse so the consumer can lift specific
// claim fields onto the gin context.
//
// The claims factory returns a freshly-allocated claims pointer per
// request. gocore can't statically know the consumer's Claims struct
// shape (iro carries IsAdmin + ProjectID + Role, mashbot has TenantID
// instead, etc.) so we accept a factory + handler instead of baking a
// concrete type into gocore.
//
// Wiring example from the consumer's main.go:
//
//	r.Use(middleware.JWTAuth(
//	    func() jwt.Claims { return new(api.Claims) },
//	    func(c *gin.Context, claims jwt.Claims) {
//	        cl := claims.(*api.Claims)
//	        c.Set("user_id", cl.UserID)
//	        if cl.IsAdmin { c.Set("is_admin", true) }
//	        // ... and any other consumer-specific lifts
//	    },
//	))
//
// Rejects the request with 401 if the header is missing/malformed or the
// token fails signature/expiry verification.
func JWTAuth(claimsFactory func() jwt.Claims, claimsHandler func(c *gin.Context, claims jwt.Claims)) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")

		claims := claimsFactory()
		if _, err := gocoreauth.VerifyToken(raw, claims); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token: " + err.Error()})
			return
		}

		if claimsHandler != nil {
			claimsHandler(c, claims)
		}

		c.Next()
	}
}
