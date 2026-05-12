// Package middleware provides Gin middleware shared across all gocore-based
// APIs: CORS, request-ID propagation, tenant context wiring, and auth.
package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS adds Cross-Origin Resource Sharing headers for the GraphQL playground and UI.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
