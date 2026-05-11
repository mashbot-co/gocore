package mixins

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mashbot-co/gocore/connection"
)

// TenantMixin adds tenant_id and auto-injects WHERE tenant_id = ? on
// query/update/delete via GORM callbacks.

func TestTenant_AutoPopulatesOnCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tenantA := uuid.New()
	ctx := connection.WithCurrentTenant(context.Background(), tenantA)

	w := &widget{Name: "tenant-A's widget"}
	if err := db.WithContext(ctx).Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.TenantID != tenantA {
		t.Fatalf("expected tenant_id=%v, got %v", tenantA, w.TenantID)
	}
}

func TestTenant_FiltersQueriesByCurrentTenant(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tenantA := uuid.New()
	tenantB := uuid.New()
	ctxA := connection.WithCurrentTenant(context.Background(), tenantA)
	ctxB := connection.WithCurrentTenant(context.Background(), tenantB)

	db.WithContext(ctxA).Create(&widget{Name: "A1"})
	db.WithContext(ctxA).Create(&widget{Name: "A2"})
	db.WithContext(ctxB).Create(&widget{Name: "B1"})

	var widgets []widget
	if err := db.WithContext(ctxA).Find(&widgets).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(widgets) != 2 {
		t.Fatalf("expected 2 widgets for tenant A, got %d", len(widgets))
	}
	for _, w := range widgets {
		if w.TenantID != tenantA {
			t.Errorf("expected tenant_id=%v, got %v", tenantA, w.TenantID)
		}
	}
}

func TestTenant_WithoutScopeBypassesFilter(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctxA := connection.WithCurrentTenant(context.Background(), uuid.New())
	ctxB := connection.WithCurrentTenant(context.Background(), uuid.New())
	db.WithContext(ctxA).Create(&widget{Name: "A1"})
	db.WithContext(ctxB).Create(&widget{Name: "B1"})

	ctx := connection.WithoutTenantScope(context.Background())
	var widgets []widget
	if err := db.WithContext(ctx).Find(&widgets).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(widgets) != 2 {
		t.Fatalf("expected 2 widgets unscoped, got %d", len(widgets))
	}
}

// --- ForTenant scope helper ---

func TestForTenant_AddsWhereClause(t *testing.T) {
	tenantID := uuid.New()
	scope := ForTenant(tenantID)
	if scope == nil {
		t.Fatal("expected non-nil scope function")
	}

	db := openSQLite(t)
	tx := scope(db.Session(&gorm.Session{NewDB: true})).Statement
	if tx == nil {
		t.Fatal("expected non-nil statement after scope application")
	}
	if _, ok := tx.Clauses["WHERE"]; !ok {
		t.Fatal("expected WHERE clause to be added by ForTenant")
	}
}

// --- isTenantedModel / hasTenantMixin detection paths ---

func TestHasTenantMixin_DetectsPointer(t *testing.T) {
	if !hasTenantMixin(&tenantedItem{}) {
		t.Fatal("expected detection via pointer")
	}
	if !hasTenantMixin(tenantedItem{}) {
		t.Fatal("expected detection via value")
	}
}

func TestHasTenantMixin_DetectsSlice(t *testing.T) {
	if !hasTenantMixin([]tenantedItem{}) {
		t.Fatal("expected detection on slice of tenanted")
	}
	if !hasTenantMixin([]*tenantedItem{}) {
		t.Fatal("expected detection on slice of *tenanted")
	}
	if hasTenantMixin([]plainItem{}) {
		t.Fatal("expected NO detection on slice of plain")
	}
}

func TestHasTenantMixin_NonStructIsFalse(t *testing.T) {
	if hasTenantMixin("not a struct") {
		t.Fatal("expected false for non-struct input")
	}
}

func TestFindTenantMixin_NilForPlainStruct(t *testing.T) {
	if got := findTenantMixin(&plainItem{}); got != nil {
		t.Fatal("expected nil for plain struct without TenantMixin")
	}
}

func TestIsTenantedModel_DetectsViaModelOnly(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Model = &tenantedItem{}
	tx.Statement.Dest = nil
	if !isTenantedModel(tx) {
		t.Fatal("expected detection via Model when Dest is nil")
	}
}

func TestIsTenantedModel_FalseWhenBothNil(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Model = nil
	tx.Statement.Dest = nil
	if isTenantedModel(tx) {
		t.Fatal("expected false when both Model and Dest are nil")
	}
}

// --- Callback early-return paths ---

func TestTenantBeforeCreate_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	item := &tenantedItem{}
	tx.Statement.Dest = item
	tenantBeforeCreate(tx)
	if item.TenantID != uuid.Nil {
		t.Fatal("expected no mutation when tx.Error is set")
	}
}

func TestTenantBeforeCreate_PreservesExplicitTenantID(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	explicit := uuid.New()
	item := &tenantedItem{TenantMixin: TenantMixin{TenantID: explicit}}
	tx.Statement.Dest = item
	tx.Statement.Context = connection.WithCurrentTenant(context.Background(), uuid.New())
	tenantBeforeCreate(tx)
	if item.TenantID != explicit {
		t.Fatalf("expected explicit tenant_id preserved, got %v", item.TenantID)
	}
}

func TestTenantBeforeQuery_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	tenantBeforeQuery(tx)
	if _, ok := tx.Statement.Clauses[clause.Where{}.Name()]; ok {
		t.Fatal("expected no WHERE clause when tx.Error is set")
	}
}

func TestTenantBeforeUpdate_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	tenantBeforeUpdate(tx) // must not panic
}

func TestTenantBeforeUpdate_NoOpWhenNotTenanted(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Model = &plainItem{}
	tenantBeforeUpdate(tx) // must not panic
}

func TestTenantBeforeDelete_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	tenantBeforeDelete(tx) // must not panic
}

func TestTenantBeforeDelete_NoOpWhenNotTenanted(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Model = &plainItem{}
	tenantBeforeDelete(tx) // must not panic
}
