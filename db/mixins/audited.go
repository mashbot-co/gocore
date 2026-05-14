package mixins

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mashbot-co/gocore/db/connection"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditEntry represents a row in a {table}_audit table.
type AuditEntry struct {
	AuditID   uuid.UUID       `gorm:"primaryKey;type:uuid" json:"audit_id"`
	RecordID  string          `gorm:"size:36;index" json:"record_id"`
	Action    string          `gorm:"size:20" json:"action"`
	ChangedAt time.Time       `gorm:"autoCreateTime;index" json:"changed_at"`
	ChangedBy *uuid.UUID      `gorm:"index;type:uuid" json:"changed_by"`
	OldValues json.RawMessage `gorm:"serializer:json" json:"old_values"`
	NewValues json.RawMessage `gorm:"serializer:json" json:"new_values"`
}

func init() {
	connection.OnInitialize(RegisterAuditCallbacks)
}

// AuditedMixin is a marker mixin. Embed it in a model to enable audit logging.
// The audit table name is derived automatically as "{table}_audit".
type AuditedMixin struct{}

// IsAudited checks whether a model embeds AuditedMixin.
func IsAudited(model interface{}) bool {
	return hasEmbeddedField(model, "AuditedMixin")
}

// AuditTableFor returns the audit table name for a given table (e.g. "users" -> "users_audit").
func AuditTableFor(table string) string {
	return table + "_audit"
}

// RegisterAuditCallbacks registers GORM callbacks for audit logging.
func RegisterAuditCallbacks(gormDB *gorm.DB) {
	gormDB.Callback().Create().After("gorm:create").Register("audit:after_create", auditAfterCreate)
	gormDB.Callback().Update().After("gorm:update").Register("audit:after_update", auditAfterUpdate)
	gormDB.Callback().Delete().After("gorm:delete").Register("audit:after_delete", auditAfterDelete)
}

func auditAfterCreate(tx *gorm.DB) {
	if tx.Error != nil || !isAuditedDest(tx) {
		return
	}
	logAudit(tx, "insert", nil, structToMap(tx.Statement.Dest))
}

func auditAfterUpdate(tx *gorm.DB) {
	if tx.Error != nil || !isAuditedDest(tx) {
		return
	}
	logAudit(tx, "update", nil, structToMap(tx.Statement.Dest))
}

func auditAfterDelete(tx *gorm.DB) {
	if tx.Error != nil || !isAuditedDest(tx) {
		return
	}
	logAudit(tx, "delete", structToMap(tx.Statement.Dest), nil)
}

func isAuditedDest(tx *gorm.DB) bool {
	if tx.Statement.Dest == nil {
		return false
	}
	return IsAudited(tx.Statement.Dest)
}

func logAudit(tx *gorm.DB, action string, oldValues, newValues map[string]any) {
	var changedBy *uuid.UUID
	if userID := connection.CurrentUser(tx.Statement.Context); userID != uuid.Nil {
		changedBy = &userID
	}

	auditTable := AuditTableFor(tx.Statement.Schema.Table)
	recordID := extractPrimaryKey(tx)

	oldJSON, _ := json.Marshal(oldValues)
	newJSON, _ := json.Marshal(newValues)

	entry := AuditEntry{
		RecordID:  recordID,
		Action:    action,
		ChangedBy: changedBy,
		OldValues: oldJSON,
		NewValues: newJSON,
	}

	tx.Session(&gorm.Session{NewDB: true}).Table(auditTable).Create(&entry)
}

// extractPrimaryKey gets the primary key value from the GORM statement.
func extractPrimaryKey(tx *gorm.DB) string {
	if tx.Statement.Schema != nil {
		for _, field := range tx.Statement.Schema.PrimaryFields {
			val, isZero := field.ValueOf(tx.Statement.Context, reflect.ValueOf(tx.Statement.Dest))
			if !isZero {
				return fmt.Sprint(val)
			}
		}
	}
	return ""
}

// skipEmbedded lists embedded mixin types to exclude from audit maps.
var skipEmbedded = map[string]bool{
	"BaseModel":       true,
	"TenantMixin":     true,
	"TrackedMixin":    true,
	"SoftDeleteMixin": true,
	"AuditedMixin":    true,
	"VersionedMixin":  true,
	"DirtyMixin":      true,
	"GraphQLMixin":    true,
}

// structToMap converts a model's fields to a map using JSON tag names.
func structToMap(dest interface{}) map[string]any {
	result := make(map[string]any)
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return result
	}
	collectFields(val, result)
	return result
}

func collectFields(val reflect.Value, result map[string]any) {
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		if !field.IsExported() {
			continue
		}

		// Skip slices (relationships)
		if field.Type.Kind() == reflect.Slice {
			continue
		}

		// Skip pointer-to-struct (relationships), but keep scalar pointer types
		if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct {
			continue
		}

		// Embedded structs: skip known mixins, recurse into others
		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			if skipEmbedded[field.Type.Name()] {
				continue
			}
			collectFields(fieldVal, result)
			continue
		}

		jsonTag := jsonKey(field)
		if jsonTag == "" {
			continue
		}
		result[jsonTag] = fieldVal.Interface()
	}
}

func jsonKey(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	if i := strings.IndexByte(tag, ','); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

func hasEmbeddedField(model interface{}, name string) bool {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	_, found := t.FieldByName(name)
	return found
}
