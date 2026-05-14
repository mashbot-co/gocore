package mixins

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mashbot-co/gocore/db/connection"
)

// setupTestDB opens a SQLite in-memory DB and registers all of gocore's mixin
// callbacks against it via connection.RegisterCallbacksOn. Each test gets a
// fresh DB so they can't interfere with each other.
//
// This helper lives here (rather than in a per-mixin test file) because every
// per-mixin test file uses it. Go's test compilation makes helpers in any
// *_test.go file visible to every other test file in the same package.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	connection.RegisterCallbacksOn(db)
	return db
}

// openSQLite is a barebones in-memory DB without callback registration. Used
// by white-box tests that want to drive specific callbacks directly without
// the global init() registrations running against the same DB.
func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// widget is the kitchen-sink test model — embeds every mixin that adds
// columns. Per-mixin tests use it because composition is the realistic case.
type widget struct {
	WidgetID uuid.UUID `gorm:"type:text;primaryKey"`
	Name     string    `gorm:"size:255;not null"`

	BaseModel
	TenantMixin
	TrackedMixin
	SoftDeleteMixin
	AuditedMixin
}

func (widget) TableName() string { return "widgets" }

// plainItem and tenantedItem cover the marker-detection paths without
// hauling along the full kitchen-sink composition.
type tenantedItem struct {
	ID uuid.UUID `gorm:"type:text;primaryKey"`
	TenantMixin
}

type projectedItem struct {
	ID uuid.UUID `gorm:"type:text;primaryKey"`
	Name string `gorm:"size:255"`
	ProjectMixin
}

func (projectedItem) TableName() string { return "projected_items" }

type plainItem struct {
	ID uuid.UUID `gorm:"type:text;primaryKey"`
}

// widgetForCallback is a minimal model used by the early-return tests on
// per-mixin callbacks — no full kitchen-sink composition needed.
type widgetForCallback struct {
	ID uuid.UUID `gorm:"type:text;primaryKey" json:"id"`
	TrackedMixin
}

// widgetWithGraphQL is the model used by GraphQLMixin detection tests.
type widgetWithGraphQL struct {
	WidgetID uuid.UUID `gorm:"type:text;primaryKey"`
	BaseModel
	GraphQLMixin
}

// --- Cross-cutting composition test ---
//
// Per-mixin behavior is tested in the paired *_test.go files. This one test
// proves the mixins compose correctly when every callback fires in one
// Create — UUID, Tenant, Tracked, Audit, plus BaseModel timestamps.

func TestComposedMixins_AllCallbacksRun(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	user := uuid.New()
	tenant := uuid.New()
	ctx := connection.WithCurrentTenant(
		connection.WithCurrentUser(context.Background(), user),
		tenant,
	)

	w := &widget{Name: "compose-test"}
	if err := db.WithContext(ctx).Create(w).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	switch {
	case w.WidgetID == uuid.Nil:
		t.Error("UUIDMixin: expected WidgetID populated")
	case w.TenantID != tenant:
		t.Errorf("TenantMixin: expected TenantID=%v, got %v", tenant, w.TenantID)
	case w.CreatedBy == nil || *w.CreatedBy != user:
		t.Errorf("TrackedMixin: expected CreatedBy=%v, got %v", user, w.CreatedBy)
	case w.CreatedAt.IsZero():
		t.Error("BaseModel: expected CreatedAt populated")
	case w.UpdatedAt.IsZero():
		t.Error("BaseModel: expected UpdatedAt populated")
	case strings.TrimSpace(w.Name) != "compose-test":
		t.Errorf("widget.Name not preserved: %q", w.Name)
	}
}
