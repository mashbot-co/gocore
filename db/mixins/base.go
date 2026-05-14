package mixins

import "time"

// BaseModel provides created_at and updated_at timestamps.
// Embed this in all models.
type BaseModel struct {
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
