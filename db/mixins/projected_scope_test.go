package mixins

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mashbot-co/gocore/db/connection"
)

// TestProject_ScopesUpdatesAndDeletes covers the WHERE-injection paths of
// projectBeforeUpdate (via Statement.Model) and projectBeforeDelete (via
// Statement.Dest) when a current project is set on the context.
func TestProject_ScopesUpdatesAndDeletes(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&projectedItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	projectA := uuid.New()
	ctxA := connection.WithCurrentProject(context.Background(), projectA)

	it := &projectedItem{ID: uuid.New(), Name: "x"}
	if err := db.WithContext(ctxA).Create(it).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update under scope → projectBeforeUpdate injects WHERE project_id.
	if err := db.WithContext(ctxA).Model(&projectedItem{}).
		Where("id = ?", it.ID).Update("name", "y").Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	// Delete under scope → projectBeforeDelete injects WHERE project_id.
	if err := db.WithContext(ctxA).
		Where("id = ?", it.ID).Delete(&projectedItem{}).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// TestIsProjectScopedModel_ViaDestAndNeither covers the Dest branch and the
// final false return.
func TestIsProjectScopedModel_ViaDestAndNeither(t *testing.T) {
	db := openSQLite(t)

	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Model = nil
	tx.Statement.Dest = &projectedItem{}
	if !isProjectScopedModel(tx) {
		t.Fatal("expected detection via Dest when Model is nil")
	}

	tx2 := db.Session(&gorm.Session{NewDB: true})
	tx2.Statement.Model = nil
	tx2.Statement.Dest = nil
	if isProjectScopedModel(tx2) {
		t.Fatal("expected false when neither Model nor Dest is set")
	}
}
