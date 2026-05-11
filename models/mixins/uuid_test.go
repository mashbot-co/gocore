package mixins

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mashbot-co/gocore/connection"
)

// --- Behavioral tests against the global UUID callback ---

func TestUUID_AutoGeneratesWhenZero(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := connection.WithCurrentTenant(context.Background(), uuid.New())
	w := &widget{Name: "test"}
	if err := db.WithContext(ctx).Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.WidgetID == uuid.Nil {
		t.Fatal("expected UUID generated, got uuid.Nil")
	}
}

func TestUUID_PreservesExplicitID(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	explicit := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	ctx := connection.WithCurrentTenant(context.Background(), uuid.New())
	w := &widget{WidgetID: explicit, Name: "test"}
	if err := db.WithContext(ctx).Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.WidgetID != explicit {
		t.Fatalf("expected explicit UUID preserved, got %v", w.WidgetID)
	}
}

// --- Early-return paths ---

func TestUUIDBeforeCreate_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	tx.Statement.Dest = &widgetForCallback{}
	uuidBeforeCreate(tx) // must not panic
}

func TestUUIDBeforeCreate_NoOpOnNilDest(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Dest = nil
	uuidBeforeCreate(tx) // must not panic
}

func TestSetUUIDPrimaryKeys_HandlesNonStruct(t *testing.T) {
	setUUIDPrimaryKeys("not a struct") // must not panic
}

// --- Pure string helpers in uuid.go ---

// uuid.go has several small helpers used by the global UUID-generation
// callback. They're worth covering directly.

func TestContainsPrimaryKey_TrueForPrimaryKeyTag(t *testing.T) {
	if !containsPrimaryKey("primaryKey;type:uuid") {
		t.Fatal("expected primaryKey to be detected in tag")
	}
	if !containsPrimaryKey("type:uuid;primaryKey") {
		t.Fatal("expected primaryKey to be detected mid-tag")
	}
}

func TestContainsPrimaryKey_FalseForOtherTags(t *testing.T) {
	if containsPrimaryKey("type:uuid;not null") {
		t.Fatal("expected non-primaryKey tag to return false")
	}
	if containsPrimaryKey("") {
		t.Fatal("expected empty tag to return false")
	}
}

func TestContains_BasicCases(t *testing.T) {
	tests := []struct {
		s, sub string
		want   bool
	}{
		{"hello world", "hello", true},
		{"hello world", "world", true},
		{"hello world", "missing", false},
		{"short", "longer-substring", false},
		{"", "anything", false},
		{"primaryKey", "primaryKey", true},
	}
	for _, c := range tests {
		if got := contains(c.s, c.sub); got != c.want {
			t.Errorf("contains(%q, %q) = %v, want %v", c.s, c.sub, got, c.want)
		}
	}
}

func TestSearchString_BasicCases(t *testing.T) {
	if !searchString("primaryKey;type:uuid", "primaryKey") {
		t.Fatal("expected match")
	}
	if searchString("primaryKey", "z") {
		t.Fatal("expected no match")
	}
}
