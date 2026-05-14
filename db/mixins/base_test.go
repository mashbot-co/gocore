package mixins

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// BaseModel is the simplest mixin: it adds CreatedAt + UpdatedAt with
// GORM's autoCreateTime/autoUpdateTime tags. These tests verify that
// embedding produces the expected fields and that GORM populates them.

func TestBaseModel_FieldsExistOnEmbed(t *testing.T) {
	type sample struct {
		ID uuid.UUID
		BaseModel
	}
	s := sample{}
	// Field presence is a compile-time guarantee — this test would fail
	// to build if BaseModel ever stopped exposing CreatedAt/UpdatedAt.
	_ = s.CreatedAt
	_ = s.UpdatedAt
}

func TestBaseModel_TimestampsPopulatedOnCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	before := time.Now().Add(-time.Second)
	w := &widget{WidgetID: uuid.New(), Name: "ts-test"}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	if w.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt populated after Create")
	}
	if w.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt populated after Create")
	}
	if w.CreatedAt.Before(before) {
		t.Errorf("CreatedAt %v is before test-start %v — clock skew or wrong autopopulate behavior", w.CreatedAt, before)
	}
}

func TestBaseModel_UpdatedAtChangesOnSave(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	w := &widget{WidgetID: uuid.New(), Name: "before"}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	initialUpdate := w.UpdatedAt

	// SQLite's time precision is coarse; sleep briefly to guarantee a
	// detectable difference.
	time.Sleep(10 * time.Millisecond)

	w.Name = "after"
	if err := db.Save(w).Error; err != nil {
		t.Fatalf("save: %v", err)
	}

	if !w.UpdatedAt.After(initialUpdate) {
		t.Errorf("expected UpdatedAt to advance, before=%v after=%v", initialUpdate, w.UpdatedAt)
	}
}
