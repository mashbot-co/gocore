package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/mashbot-co/gocore/models/mixins"
)

// Agent is a registered AI agent. Its description spans
// multiple lines for the parser to fold into a single GraphQL description.
type Agent struct {
	AgentID     uuid.UUID  `gorm:"primaryKey;type:uuid" json:"agent_id"`       // Unique identifier
	Name        string     `gorm:"size:255;not null" json:"name"`              // Display name
	Slug        string     `gorm:"size:100;not null" json:"slug"`              // URL slug
	Description *string    `gorm:"type:text" json:"description,omitempty"`     // Optional description
	OwnerID     uuid.UUID  `gorm:"index;type:uuid;not null" json:"owner_id"`   // FK to user
	ArchivedAt  *time.Time `gorm:"" json:"archived_at,omitempty"`              // Soft archive

	mixins.BaseModel
	mixins.TenantMixin
	mixins.TrackedMixin
	mixins.SoftDeleteMixin
	mixins.GraphQLMixin

	Owner    *User           `gorm:"foreignKey:OwnerID" json:"owner,omitempty" graphql:"preload"`
	Versions []AgentVersion  `gorm:"foreignKey:AgentID" json:"versions,omitempty" graphql:"preload"`
	Hidden   string          `gorm:"-" json:"hidden,omitempty" graphql:"-"`
	ReadOnly string          `gorm:"-" json:"readonly,omitempty" graphql:"readonly"`
}
