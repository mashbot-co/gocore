package mixins

import (
	"time"

	"github.com/google/uuid"
)

// VersionedMixin adds versioning support for immutable, append-only version records.
// Embed in version models (AgentVersion, SkillVersion, PlaybookVersion).
//
// Status values vary by entity type (enforced at the application layer):
//   - AgentVersion: draft → testing → canary → active → retired
//   - SkillVersion / PlaybookVersion: draft → active → retired
type VersionedMixin struct {
	VersionLabel      string     `gorm:"size:50;not null" json:"version_label"`
	Status            string     `gorm:"size:50;not null;default:'draft'" json:"status"`
	AuthoredBy        uuid.UUID  `gorm:"index;type:uuid;not null" json:"authored_by"`
	ChangeSummary     *string    `gorm:"type:text" json:"change_summary,omitempty"`
	PolicyCheckStatus *string    `gorm:"size:50" json:"policy_check_status,omitempty"`
	PolicyCheckAt     *time.Time `gorm:"" json:"policy_check_at,omitempty"`
	PolicyCheckNote   *string    `gorm:"type:text" json:"policy_check_note,omitempty"`
}
