package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore/db/connection"
)

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
// the user ID for GORM's tracked-by callbacks. Use when you want to thread the
// value directly into a one-off GORM call. Consumers that also scope by tenant
// / project / workspace add their own middleware that calls the matching
// connection.WithCurrent* helper after this one.
func GormContext(c *gin.Context) *gin.Context {
	if userID := UserIDFromContext(c); userID != uuid.Nil {
		c.Request = c.Request.WithContext(connection.WithCurrentUser(c.Request.Context(), userID))
	}
	return c
}

// InjectDBContext is a Gin middleware that sets the user ID on the request
// context so that GORM's TrackedMixin callbacks can populate created_by /
// updated_by. Place this after Auth middleware. Scope-specific middleware
// (tenant, project, etc) layers on top in consumer code.
func InjectDBContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		if userID := UserIDFromContext(c); userID != uuid.Nil {
			c.Request = c.Request.WithContext(connection.WithCurrentUser(c.Request.Context(), userID))
		}
		c.Next()
	}
}
