package models

import (
	"github.com/google/uuid"
	"github.com/mashbot-co/gocore/db/mixins"
)

// AgentVersion represents one immutable version of an Agent.
type AgentVersion struct {
	AgentVersionID uuid.UUID `gorm:"primaryKey;type:uuid" json:"agent_version_id"`
	AgentID        uuid.UUID `gorm:"index;type:uuid;not null" json:"agent_id"`

	mixins.BaseModel
	mixins.VersionedMixin
	mixins.GraphQLMixin
}

// Promote moves a draft version into active production status.
// Demonstrates an instance-method GraphQL mutation that the
// parser picks up via parseMutationAnnotation.
//
// graphql:mutation:promote(targetStatus string, note string)
func (v *AgentVersion) Promote() error {
	return nil
}

// Archive marks the version archived. Annotation without parameters
// covers the path where the parser sees graphql:mutation with no
// trailing parentheses.
//
// graphql:mutation
func (v *AgentVersion) Archive() error {
	return nil
}
