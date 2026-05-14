package mixins

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SoftDeleteMixin adds soft delete support using GORM's built-in mechanism.
type SoftDeleteMixin struct {
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	DeletedBy *uuid.UUID `json:"deleted_by"`
}

// WithDeleted returns a scope that includes soft-deleted records.
func WithDeleted(db *gorm.DB) *gorm.DB {
	return db.Unscoped()
}

// OnlyDeleted returns a scope for only soft-deleted records.
func OnlyDeleted(db *gorm.DB) *gorm.DB {
	return db.Unscoped().Where("deleted_at IS NOT NULL")
}

// Restore un-deletes a soft-deleted record.
func Restore(db *gorm.DB, model interface{}) error {
	return db.Unscoped().Model(model).Updates(map[string]interface{}{
		"deleted_at": nil,
		"deleted_by": nil,
	}).Error
}
