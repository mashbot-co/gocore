package mixins

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type softWidget struct {
	WidgetID uuid.UUID `gorm:"type:text;primaryKey"`
	Name     string    `gorm:"size:255"`

	BaseModel
	SoftDeleteMixin
}

func (softWidget) TableName() string { return "soft_widgets" }

func openSoftSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&softWidget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSoftDelete_WithDeletedReturnsAll(t *testing.T) {
	db := openSoftSQLite(t)

	live := &softWidget{WidgetID: uuid.New(), Name: "live"}
	dead := &softWidget{WidgetID: uuid.New(), Name: "dead"}
	db.Create(live)
	db.Create(dead)
	db.Delete(dead)

	var widgets []softWidget
	if err := WithDeleted(db).Find(&widgets).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(widgets) != 2 {
		t.Fatalf("expected 2 widgets via WithDeleted, got %d", len(widgets))
	}
}

func TestSoftDelete_OnlyDeletedReturnsTombstones(t *testing.T) {
	db := openSoftSQLite(t)

	live := &softWidget{WidgetID: uuid.New(), Name: "live"}
	dead := &softWidget{WidgetID: uuid.New(), Name: "dead"}
	db.Create(live)
	db.Create(dead)
	db.Delete(dead)

	var widgets []softWidget
	if err := OnlyDeleted(db).Find(&widgets).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(widgets) != 1 {
		t.Fatalf("expected 1 deleted widget, got %d", len(widgets))
	}
	if widgets[0].Name != "dead" {
		t.Fatalf("expected name=dead, got %q", widgets[0].Name)
	}
}

func TestSoftDelete_RestoreClearsTombstone(t *testing.T) {
	db := openSoftSQLite(t)

	w := &softWidget{WidgetID: uuid.New(), Name: "restore-me"}
	db.Create(w)
	db.Delete(w)

	// Verify the tombstone is set.
	var beforeRestore softWidget
	if err := WithDeleted(db).First(&beforeRestore, "widget_id = ?", w.WidgetID).Error; err != nil {
		t.Fatalf("load pre-restore: %v", err)
	}
	if !beforeRestore.DeletedAt.Valid {
		t.Fatal("expected DeletedAt to be set after Delete")
	}

	if err := Restore(db, &softWidget{WidgetID: w.WidgetID}); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// After restore, normal Find should see the row again.
	var afterRestore []softWidget
	if err := db.Find(&afterRestore).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(afterRestore) != 1 {
		t.Fatalf("expected 1 visible widget after Restore, got %d", len(afterRestore))
	}
}
