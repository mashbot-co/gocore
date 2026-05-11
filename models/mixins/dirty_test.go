package mixins

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// dirtyWidget is the test model for DirtyMixin. White-box (package mixins)
// because we need access to the unexported snapshot field for some cases.
type dirtyWidget struct {
	WidgetID uuid.UUID `gorm:"type:text;primaryKey"`
	Name     string    `gorm:"size:255" json:"name"`
	Count    int       `gorm:"" json:"count"`

	BaseModel
	DirtyMixin
}

func (dirtyWidget) TableName() string { return "dirty_widgets" }

func openDirtySQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Install DirtyMixin's after-find/after-save callbacks via the connection
	// hook. (See dirty.go init.)
	RegisterDirtyCallbacks(db)
	if err := db.AutoMigrate(&dirtyWidget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// --- Without snapshot: IsDirty / DirtyFields return safely ---

func TestDirty_NoSnapshotReturnsNotDirty(t *testing.T) {
	w := &dirtyWidget{Name: "fresh"}
	if w.IsDirty() {
		t.Fatal("expected IsDirty=false on fresh struct (no snapshot taken)")
	}
	if got := w.DirtyFields(); got != nil {
		t.Fatalf("expected nil DirtyFields, got %v", got)
	}
}

// --- After load: no changes means not dirty ---

func TestDirty_AfterLoadNoChangesNotDirty(t *testing.T) {
	db := openDirtySQLite(t)

	original := &dirtyWidget{WidgetID: uuid.New(), Name: "original", Count: 1}
	if err := db.Create(original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var loaded dirtyWidget
	if err := db.First(&loaded, "widget_id = ?", original.WidgetID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.IsDirty() {
		t.Fatalf("expected not-dirty after load with no changes; got DirtyFields=%v", loaded.DirtyFields())
	}
}

// --- Mutate after load: IsDirty true, fields enumerated ---

func TestDirty_DetectsFieldChanges(t *testing.T) {
	db := openDirtySQLite(t)

	original := &dirtyWidget{WidgetID: uuid.New(), Name: "before", Count: 1}
	if err := db.Create(original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var loaded dirtyWidget
	if err := db.First(&loaded, "widget_id = ?", original.WidgetID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}

	loaded.Name = "after"
	loaded.Count = 2

	if !loaded.IsDirty() {
		t.Fatal("expected IsDirty=true after mutation")
	}

	fields := loaded.DirtyFields()
	hasName, hasCount := false, false
	for _, f := range fields {
		switch f {
		case "name":
			hasName = true
		case "count":
			hasCount = true
		}
	}
	if !hasName || !hasCount {
		t.Fatalf("expected 'name' and 'count' in DirtyFields, got %v", fields)
	}
}

// --- OriginalValue returns the pre-mutation value ---

func TestDirty_OriginalValueReturnsSnapshot(t *testing.T) {
	db := openDirtySQLite(t)

	original := &dirtyWidget{WidgetID: uuid.New(), Name: "before"}
	if err := db.Create(original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var loaded dirtyWidget
	if err := db.First(&loaded, "widget_id = ?", original.WidgetID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}

	loaded.Name = "after"

	if got := loaded.OriginalValue("name"); got != "before" {
		t.Fatalf("expected original value 'before', got %v", got)
	}

	if got := loaded.OriginalValue("does_not_exist"); got != nil {
		t.Fatalf("expected nil for unknown field, got %v", got)
	}
}

// --- Changes returns a before/after map per changed field ---

func TestDirty_ChangesMap(t *testing.T) {
	db := openDirtySQLite(t)

	original := &dirtyWidget{WidgetID: uuid.New(), Name: "before", Count: 1}
	if err := db.Create(original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var loaded dirtyWidget
	if err := db.First(&loaded, "widget_id = ?", original.WidgetID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}

	loaded.Name = "after"

	changes := loaded.Changes()
	change, ok := changes["name"]
	if !ok {
		t.Fatalf("expected 'name' in changes map; got %v", changes)
	}
	if change.Old != "before" || change.New != "after" {
		t.Fatalf("expected Old=before/New=after, got %+v", change)
	}
}

// --- ResetDirty clears the snapshot ---

func TestDirty_ResetDirtyClearsSnapshot(t *testing.T) {
	db := openDirtySQLite(t)

	original := &dirtyWidget{WidgetID: uuid.New(), Name: "before"}
	if err := db.Create(original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var loaded dirtyWidget
	if err := db.First(&loaded, "widget_id = ?", original.WidgetID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}

	loaded.Name = "after"
	if !loaded.IsDirty() {
		t.Fatal("expected dirty before reset")
	}

	loaded.ResetDirty()
	if loaded.IsDirty() {
		t.Fatal("expected not-dirty after ResetDirty")
	}
}

// --- After save: callback should re-snapshot (so further changes are detected) ---

func TestDirty_SaveResnapshots(t *testing.T) {
	db := openDirtySQLite(t)

	w := &dirtyWidget{WidgetID: uuid.New(), Name: "first"}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// After load, mutate, save — the snapshot should reflect the saved state.
	var loaded dirtyWidget
	if err := db.First(&loaded, "widget_id = ?", w.WidgetID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Name = "second"
	if err := db.Save(&loaded).Error; err != nil {
		t.Fatalf("save: %v", err)
	}

	// Immediately after save the row is no longer dirty.
	if loaded.IsDirty() {
		t.Fatalf("expected not-dirty after Save; got DirtyFields=%v", loaded.DirtyFields())
	}

	// New mutation should be detected.
	loaded.Name = "third"
	if !loaded.IsDirty() {
		t.Fatal("expected dirty after post-save mutation")
	}
	if !strings.EqualFold(loaded.DirtyFields()[0], "name") {
		t.Fatalf("expected 'name' in DirtyFields, got %v", loaded.DirtyFields())
	}
}

// --- Reflection / early-return paths ---

func TestDirtyAfterFind_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	dirtyAfterFind(tx) // must not panic
}

func TestDirtyAfterUpdate_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	dirtyAfterUpdate(tx) // must not panic
}

func TestFindDirtyMixin_NilForPlainStruct(t *testing.T) {
	if got := findDirtyMixin(&plainItem{}); got != nil {
		t.Fatal("expected nil for plain struct without DirtyMixin")
	}
}

func TestChanges_NoSnapshotReturnsNil(t *testing.T) {
	d := &DirtyMixin{}
	if got := d.Changes(); got != nil {
		t.Fatalf("expected nil Changes() when no snapshot, got %v", got)
	}
}

func TestCurrentValues_NonStructIsEmpty(t *testing.T) {
	d := &DirtyMixin{parent: "not a struct"}
	got := d.currentValues()
	if len(got) != 0 {
		t.Fatalf("expected empty currentValues for non-struct parent, got %v", got)
	}
}

func TestSnapshotDest_NonStructDoesNotPanic(t *testing.T) {
	d := &DirtyMixin{}
	d.takeSnapshot("not a struct")
}
