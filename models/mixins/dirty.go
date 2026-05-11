package mixins

import (
	"reflect"
	"sync"

	"github.com/mashbot-co/gocore/connection"

	"gorm.io/gorm"
)

func init() {
	connection.OnInitialize(RegisterDirtyCallbacks)
}

// DirtyMixin provides change tracking for models. Embed in any model
// that needs to detect which fields have been modified since load.
//
// Usage:
//
//	agent := // loaded from DB via Find, First, etc.
//	agent.Name = "New Name"
//	agent.IsDirty()              // true
//	agent.DirtyFields()          // ["name"]
//	agent.OriginalValue("name")  // "Old Name"
//	agent.Changes()              // map["name"]{Old: "Old Name", New: "New Name"}
//	agent.ResetDirty()           // clears the snapshot (called automatically after save)
type DirtyMixin struct {
	snapshot map[string]any `gorm:"-" json:"-"`
	parent   interface{}    `gorm:"-" json:"-"`
	mu       sync.RWMutex  `gorm:"-" json:"-"`
}

// FieldChange represents a before/after pair for a changed field.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// IsDirty returns true if any field has been modified since the record was loaded.
func (d *DirtyMixin) IsDirty() bool {
	return len(d.DirtyFields()) > 0
}

// DirtyFields returns the JSON tag names of all fields that have changed since load.
func (d *DirtyMixin) DirtyFields() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.snapshot == nil {
		return nil
	}

	var dirty []string
	current := d.currentValues()
	for key, oldVal := range d.snapshot {
		if newVal, exists := current[key]; exists {
			if !reflect.DeepEqual(oldVal, newVal) {
				dirty = append(dirty, key)
			}
		}
	}
	return dirty
}

// OriginalValue returns the value of a field as it was when loaded from the database.
// The key is the JSON tag name (e.g., "name", "slug").
func (d *DirtyMixin) OriginalValue(jsonKey string) any {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.snapshot == nil {
		return nil
	}
	return d.snapshot[jsonKey]
}

// Changes returns a map of all changed fields with their old and new values.
func (d *DirtyMixin) Changes() map[string]FieldChange {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.snapshot == nil {
		return nil
	}

	changes := make(map[string]FieldChange)
	current := d.currentValues()
	for key, oldVal := range d.snapshot {
		if newVal, exists := current[key]; exists {
			if !reflect.DeepEqual(oldVal, newVal) {
				changes[key] = FieldChange{Old: oldVal, New: newVal}
			}
		}
	}
	return changes
}

// ResetDirty clears the snapshot and re-captures the current state.
// Called automatically after successful create/update operations.
func (d *DirtyMixin) ResetDirty() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.parent != nil {
		d.snapshot = extractFieldValues(d.parent)
	}
}

// takeSnapshot captures the current field values of the parent struct
// and stores a reference to the parent for live comparison.
func (d *DirtyMixin) takeSnapshot(dest interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.parent = dest
	d.snapshot = extractFieldValues(dest)
}

// currentValues extracts the live field values from the parent struct.
func (d *DirtyMixin) currentValues() map[string]any {
	if d.parent == nil {
		return d.snapshot
	}
	return extractFieldValues(d.parent)
}

// extractFieldValues walks a struct and extracts field values keyed by JSON tag.
func extractFieldValues(dest interface{}) map[string]any {
	result := make(map[string]any)
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return result
	}
	collectFieldValues(val, result)
	return result
}

func collectFieldValues(val reflect.Value, result map[string]any) {
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

		// Skip pointer-to-struct (relationships)
		if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct {
			// But keep *time.Time and *uuid.UUID (scalar wrappers)
			elemName := field.Type.Elem().Name()
			if elemName != "Time" && elemName != "UUID" {
				continue
			}
		}

		// Skip known mixin types and sync primitives
		if field.Anonymous {
			typeName := field.Type.Name()
			if skipDirtyEmbedded[typeName] {
				continue
			}
			if fieldVal.Kind() == reflect.Struct {
				collectFieldValues(fieldVal, result)
				continue
			}
		}

		tag := jsonKey(field)
		if tag == "" {
			continue
		}

		result[tag] = fieldVal.Interface()
	}
}

var skipDirtyEmbedded = map[string]bool{
	"BaseModel":       true,
	"TenantMixin":     true,
	"TrackedMixin":    true,
	"SoftDeleteMixin": true,
	"AuditedMixin":    true,
	"VersionedMixin":  true,
	"DirtyMixin":      true,
	"GraphQLMixin":    true,
	"Mutex":           true,
	"RWMutex":         true,
}

// RegisterDirtyCallbacks registers GORM callbacks that snapshot field values
// after loading and reset dirty state after saving.
func RegisterDirtyCallbacks(gormDB *gorm.DB) {
	gormDB.Callback().Query().After("gorm:query").Register("dirty:after_find", dirtyAfterFind)
	gormDB.Callback().Create().After("gorm:create").Register("dirty:after_create", dirtyAfterCreate)
	gormDB.Callback().Update().After("gorm:update").Register("dirty:after_update", dirtyAfterUpdate)
}

func dirtyAfterFind(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	snapshotDest(tx.Statement.Dest)
}

func dirtyAfterCreate(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	snapshotDest(tx.Statement.Dest)
}

func dirtyAfterUpdate(tx *gorm.DB) {
	if tx.Error != nil || tx.Statement.Dest == nil {
		return
	}
	snapshotDest(tx.Statement.Dest)
}

func snapshotDest(dest interface{}) {
	// Handle single struct
	if d := findDirtyMixin(dest); d != nil {
		d.takeSnapshot(dest)
		return
	}

	// Handle slice of structs (e.g., Find(&agents))
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() == reflect.Slice {
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i)
			if elem.Kind() == reflect.Ptr {
				elem = elem.Elem()
			}
			if elem.CanAddr() {
				if d := findDirtyMixin(elem.Addr().Interface()); d != nil {
					d.takeSnapshot(elem.Addr().Interface())
				}
			}
		}
	}
}

// findDirtyMixin uses reflection to find an embedded DirtyMixin in a model.
func findDirtyMixin(dest interface{}) *DirtyMixin {
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}
	field := val.FieldByName("DirtyMixin")
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	if dirty, ok := field.Addr().Interface().(*DirtyMixin); ok {
		return dirty
	}
	return nil
}
