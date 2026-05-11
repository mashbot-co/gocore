package mixins

import (
	"reflect"

	"github.com/mashbot-co/gocore/connection"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func init() {
	connection.OnInitialize(RegisterUUIDCallbacks)
}

// RegisterUUIDCallbacks registers a GORM callback that auto-generates UUIDs
// for any primaryKey field of type uuid.UUID that is still zero at create time.
// This replaces the Postgres-specific DEFAULT gen_random_uuid() with a
// dialect-agnostic Go-side UUID generator.
func RegisterUUIDCallbacks(gormDB *gorm.DB) {
	gormDB.Callback().Create().Before("gorm:create").Register("uuid:before_create", uuidBeforeCreate)
}

func uuidBeforeCreate(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	setUUIDPrimaryKeys(tx.Statement.Dest)
}

// setUUIDPrimaryKeys finds all uuid.UUID fields tagged as primaryKey and
// generates a new UUID if the current value is zero.
func setUUIDPrimaryKeys(dest interface{}) {
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return
	}

	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// Check embedded structs recursively
		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			setUUIDPrimaryKeys(fieldVal.Addr().Interface())
			continue
		}

		// Only process uuid.UUID fields
		if field.Type != reflect.TypeOf(uuid.UUID{}) {
			continue
		}

		// Check if it's a primary key via gorm tag
		tag := field.Tag.Get("gorm")
		if !containsPrimaryKey(tag) {
			continue
		}

		// If zero, generate a new UUID
		if fieldVal.Interface().(uuid.UUID) == uuid.Nil {
			fieldVal.Set(reflect.ValueOf(uuid.New()))
		}
	}
}

func containsPrimaryKey(tag string) bool {
	// Simple check — the tag contains "primaryKey" (case-sensitive, matches GORM convention)
	return len(tag) > 0 && contains(tag, "primaryKey")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
