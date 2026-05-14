package mixins

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mashbot-co/gocore/db/connection"
)

// ProjectMixin mirrors TenantMixin but scopes by project_id. These tests
// exercise the same callback contract: auto-populate on create, inject
// WHERE on query/update/delete, and respect explicit overrides.

func TestProject_AutoPopulatesOnCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&projectedItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	projectA := uuid.New()
	ctx := connection.WithCurrentProject(context.Background(), projectA)

	p := &projectedItem{ID: uuid.New(), Name: "project-A's thing"}
	if err := db.WithContext(ctx).Create(p).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ProjectID != projectA {
		t.Fatalf("expected project_id=%v, got %v", projectA, p.ProjectID)
	}
}

func TestProject_FiltersQueriesByCurrentProject(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&projectedItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	projectA := uuid.New()
	projectB := uuid.New()
	ctxA := connection.WithCurrentProject(context.Background(), projectA)
	ctxB := connection.WithCurrentProject(context.Background(), projectB)

	db.WithContext(ctxA).Create(&projectedItem{ID: uuid.New(), Name: "A1"})
	db.WithContext(ctxA).Create(&projectedItem{ID: uuid.New(), Name: "A2"})
	db.WithContext(ctxB).Create(&projectedItem{ID: uuid.New(), Name: "B1"})

	var items []projectedItem
	if err := db.WithContext(ctxA).Find(&items).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items for project A, got %d", len(items))
	}
	for _, it := range items {
		if it.ProjectID != projectA {
			t.Errorf("expected project_id=%v, got %v", projectA, it.ProjectID)
		}
	}
}

func TestProject_WithoutScopeBypassesFilter(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&projectedItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctxA := connection.WithCurrentProject(context.Background(), uuid.New())
	ctxB := connection.WithCurrentProject(context.Background(), uuid.New())
	db.WithContext(ctxA).Create(&projectedItem{ID: uuid.New(), Name: "A1"})
	db.WithContext(ctxB).Create(&projectedItem{ID: uuid.New(), Name: "B1"})

	ctx := connection.WithoutProjectScope(context.Background())
	var items []projectedItem
	if err := db.WithContext(ctx).Find(&items).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items unscoped, got %d", len(items))
	}
}

func TestForProject_AddsWhereClause(t *testing.T) {
	projectID := uuid.New()
	scope := ForProject(projectID)
	if scope == nil {
		t.Fatal("expected non-nil scope function")
	}

	db := openSQLite(t)
	tx := scope(db.Session(&gorm.Session{NewDB: true})).Statement
	if tx == nil {
		t.Fatal("expected non-nil statement after scope application")
	}
	if _, ok := tx.Clauses["WHERE"]; !ok {
		t.Fatal("expected WHERE clause to be added by ForProject")
	}
}

func TestHasProjectMixin_DetectsPointerAndValueAndSlice(t *testing.T) {
	if !hasProjectMixin(&projectedItem{}) {
		t.Fatal("expected detection via pointer")
	}
	if !hasProjectMixin(projectedItem{}) {
		t.Fatal("expected detection via value")
	}
	if !hasProjectMixin([]projectedItem{}) {
		t.Fatal("expected detection on slice of projected")
	}
	if !hasProjectMixin([]*projectedItem{}) {
		t.Fatal("expected detection on slice of *projected")
	}
	if hasProjectMixin([]plainItem{}) {
		t.Fatal("expected NO detection on slice of plain")
	}
	if hasProjectMixin("not a struct") {
		t.Fatal("expected false for non-struct input")
	}
}

func TestFindProjectMixin_NilForPlainStruct(t *testing.T) {
	if got := findProjectMixin(&plainItem{}); got != nil {
		t.Fatal("expected nil for plain struct without ProjectMixin")
	}
}

func TestIsProjectScopedModel_DetectsViaModelOnly(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Model = &projectedItem{}
	tx.Statement.Dest = nil
	if !isProjectScopedModel(tx) {
		t.Fatal("expected detection via Model when Dest is nil")
	}
}

func TestProjectBeforeCreate_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	item := &projectedItem{}
	tx.Statement.Dest = item
	projectBeforeCreate(tx)
	if item.ProjectID != uuid.Nil {
		t.Fatal("expected no mutation when tx.Error is set")
	}
}

func TestProjectBeforeCreate_PreservesExplicitProjectID(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	explicit := uuid.New()
	item := &projectedItem{ProjectMixin: ProjectMixin{ProjectID: explicit}}
	tx.Statement.Dest = item
	tx.Statement.Context = connection.WithCurrentProject(context.Background(), uuid.New())
	projectBeforeCreate(tx)
	if item.ProjectID != explicit {
		t.Fatalf("expected explicit project_id preserved, got %v", item.ProjectID)
	}
}

func TestProjectBeforeQuery_NoOpOnError(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	projectBeforeQuery(tx)
	if _, ok := tx.Statement.Clauses[clause.Where{}.Name()]; ok {
		t.Fatal("expected no WHERE clause when tx.Error is set")
	}
}

func TestProjectBeforeUpdate_NoOpOnErrorOrUnscopedModel(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	projectBeforeUpdate(tx)

	tx2 := db.Session(&gorm.Session{NewDB: true})
	tx2.Statement.Model = &plainItem{}
	projectBeforeUpdate(tx2)
}

func TestProjectBeforeDelete_NoOpOnErrorOrUnscopedModel(t *testing.T) {
	db := openSQLite(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Error = errors.New("forced")
	projectBeforeDelete(tx)

	tx2 := db.Session(&gorm.Session{NewDB: true})
	tx2.Statement.Model = &plainItem{}
	projectBeforeDelete(tx2)
}
