package mixins

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mashbot-co/gocore/db/connection"
)

// TrackedMixin adds created_by + updated_by and populates them from the
// current user in request context via GORM before-create/before-update
// callbacks.

func TestTracked_PopulatesCreatedByOnCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	user := uuid.New()
	ctx := connection.WithCurrentUser(context.Background(), user)
	ctx = connection.WithCurrentTenant(ctx, uuid.New())

	w := &widget{Name: "test"}
	if err := db.WithContext(ctx).Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.CreatedBy == nil || *w.CreatedBy != user {
		t.Fatalf("expected CreatedBy=%v, got %v", user, w.CreatedBy)
	}
	if w.UpdatedBy == nil || *w.UpdatedBy != user {
		t.Fatalf("expected UpdatedBy=%v, got %v", user, w.UpdatedBy)
	}
}

func TestTracked_PopulatesUpdatedByOnSave(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tenant := uuid.New()
	creator := uuid.New()
	updater := uuid.New()
	ctxCreate := connection.WithCurrentTenant(
		connection.WithCurrentUser(context.Background(), creator), tenant)
	ctxUpdate := connection.WithCurrentTenant(
		connection.WithCurrentUser(context.Background(), updater), tenant)

	w := &widget{Name: "original"}
	if err := db.WithContext(ctxCreate).Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// Save() emits the full struct in the UPDATE; column-specific Update()
	// would lose the mixin's in-memory mutation. Documented in the mixin
	// godoc.
	w.Name = "modified"
	if err := db.WithContext(ctxUpdate).Save(w).Error; err != nil {
		t.Fatalf("save: %v", err)
	}

	var fresh widget
	if err := db.WithContext(ctxUpdate).First(&fresh, "widget_id = ?", w.WidgetID).Error; err != nil {
		t.Fatalf("reread: %v", err)
	}
	if fresh.UpdatedBy == nil || *fresh.UpdatedBy != updater {
		t.Fatalf("expected UpdatedBy=%v, got %v", updater, fresh.UpdatedBy)
	}
	if fresh.CreatedBy == nil || *fresh.CreatedBy != creator {
		t.Fatalf("expected CreatedBy=%v (unchanged), got %v", creator, fresh.CreatedBy)
	}
}

// --- Early-return paths ---

func TestTrackedBeforeCreate_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	tx.Statement.Dest = &widgetForCallback{}
	trackedBeforeCreate(tx)
	if w, ok := tx.Statement.Dest.(*widgetForCallback); ok && w.CreatedBy != nil {
		t.Fatal("expected no mutation when tx.Error is set")
	}
}

func TestTrackedBeforeCreate_NoOpOnNilDest(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Dest = nil
	trackedBeforeCreate(tx) // must not panic
}

func TestTrackedBeforeCreate_NoOpWhenNoUserInContext(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	w := &widgetForCallback{}
	tx.Statement.Dest = w
	tx.Statement.Context = context.Background() // no user
	trackedBeforeCreate(tx)
	if w.CreatedBy != nil {
		t.Fatal("expected CreatedBy nil when context has no user")
	}
}

func TestTrackedBeforeUpdate_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	tx.Statement.Dest = &widgetForCallback{}
	trackedBeforeUpdate(tx) // must not panic
}

func TestTrackedBeforeUpdate_NoOpWhenNoTrackedMixin(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Dest = &plainItem{}
	tx.Statement.Context = connection.WithCurrentUser(context.Background(), uuid.New())
	trackedBeforeUpdate(tx) // must not panic
}

// --- Reflection helper ---

func TestFindTrackedMixin_NilForPlainStruct(t *testing.T) {
	if got := findTrackedMixin(&plainItem{}); got != nil {
		t.Fatal("expected nil for plain struct without TrackedMixin")
	}
}
