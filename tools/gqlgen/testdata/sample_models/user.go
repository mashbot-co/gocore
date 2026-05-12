package models

import (
	"github.com/google/uuid"
	"github.com/mashbot-co/gocore/models/mixins"
)

// User is a person who uses the system.
type User struct {
	UserID uuid.UUID `gorm:"primaryKey;type:uuid" json:"user_id"`
	Email  string    `gorm:"size:255;not null;uniqueIndex" json:"email" graphql:"filterable"`

	mixins.BaseModel
	mixins.TenantMixin
	mixins.GraphQLMixin
}
