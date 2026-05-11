package mixins

import (
	"reflect"

	"github.com/mashbot-co/gocore/connection"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func init() {
	connection.OnInitialize(RegisterTrackedCallbacks)
}

// TrackedMixin adds created_by and updated_by fields.
type TrackedMixin struct {
	CreatedBy *uuid.UUID `json:"created_by"`
	UpdatedBy *uuid.UUID `json:"updated_by"`
}

// RegisterTrackedCallbacks registers GORM callbacks that automatically set
// created_by/updated_by from the request context on any model embedding TrackedMixin.
func RegisterTrackedCallbacks(gormDB *gorm.DB) {
	gormDB.Callback().Create().Before("gorm:create").Register("tracked:before_create", trackedBeforeCreate)
	gormDB.Callback().Update().Before("gorm:update").Register("tracked:before_update", trackedBeforeUpdate)
}

func trackedBeforeCreate(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	tracked := findTrackedMixin(tx.Statement.Dest)
	if tracked == nil {
		return
	}
	if userID := connection.CurrentUser(tx.Statement.Context); userID != uuid.Nil {
		tracked.CreatedBy = &userID
		tracked.UpdatedBy = &userID
	}
}

func trackedBeforeUpdate(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	tracked := findTrackedMixin(tx.Statement.Dest)
	if tracked == nil {
		return
	}
	if userID := connection.CurrentUser(tx.Statement.Context); userID != uuid.Nil {
		tracked.UpdatedBy = &userID
	}
}

// findTrackedMixin uses reflection to find an embedded TrackedMixin in a model.
func findTrackedMixin(dest interface{}) *TrackedMixin {
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}
	field := val.FieldByName("TrackedMixin")
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	if tracked, ok := field.Addr().Interface().(*TrackedMixin); ok {
		return tracked
	}
	return nil
}
