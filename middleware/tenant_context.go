package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore/connection"
)

// TenantIDFromContext extracts the tenant_id set by the Auth middleware.
func TenantIDFromContext(c *gin.Context) uuid.UUID {
	if v, exists := c.Get("tenant_id"); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// UserIDFromContext extracts the user_id set by the Auth middleware.
func UserIDFromContext(c *gin.Context) uuid.UUID {
	if v, exists := c.Get("user_id"); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// GormContext mutates the gin.Context's underlying *http.Request so it carries
// the tenant ID and user ID for GORM's automatic scoping callbacks. Use when
// you want to thread the values directly into a one-off GORM call.
func GormContext(c *gin.Context) *gin.Context {
	ctx := c.Request.Context()
	tenantID := TenantIDFromContext(c)
	userID := UserIDFromContext(c)

	if tenantID != uuid.Nil {
		ctx = connection.WithCurrentTenant(ctx, tenantID)
	}
	if userID != uuid.Nil {
		ctx = connection.WithCurrentUser(ctx, userID)
	}

	c.Request = c.Request.WithContext(ctx)
	return c
}

// InjectDBContext is a Gin middleware that sets the tenant and user IDs
// on the request context so that GORM callbacks can automatically apply
// tenant scoping and user tracking. Place this after Auth middleware.
func InjectDBContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := TenantIDFromContext(c)
		userID := UserIDFromContext(c)

		if tenantID != uuid.Nil {
			ctx = connection.WithCurrentTenant(ctx, tenantID)
		}
		if userID != uuid.Nil {
			ctx = connection.WithCurrentUser(ctx, userID)
		}

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
