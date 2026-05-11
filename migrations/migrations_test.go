package migrations

import (
	"testing"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// resetRegistry wipes the global migration registry. Tests register and
// unregister their own migrations so they don't leak into each other.
func resetRegistry() func() {
	saved := make([]*gormigrate.Migration, len(registry))
	copy(saved, registry)
	registry = nil
	return func() {
		registry = saved
	}
}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// --- Register ---

func TestRegister_AcceptsValidMigration(t *testing.T) {
	defer resetRegistry()()

	Register(&gormigrate.Migration{
		ID:       "20260101000001",
		Migrate:  func(tx *gorm.DB) error { return nil },
		Rollback: func(tx *gorm.DB) error { return nil },
	})

	if len(registry) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(registry))
	}
}

func TestRegister_PanicsOnInvalidID(t *testing.T) {
	defer resetRegistry()()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on non-14-digit ID")
		}
	}()

	Register(&gormigrate.Migration{
		ID:       "not-a-timestamp",
		Migrate:  func(tx *gorm.DB) error { return nil },
		Rollback: func(tx *gorm.DB) error { return nil },
	})
}

func TestRegister_PanicsWhenMissingMigrate(t *testing.T) {
	defer resetRegistry()()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when Migrate is nil")
		}
	}()

	Register(&gormigrate.Migration{
		ID:       "20260101000001",
		Rollback: func(tx *gorm.DB) error { return nil },
	})
}

func TestRegister_PanicsWhenMissingRollback(t *testing.T) {
	defer resetRegistry()()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when Rollback is nil")
		}
	}()

	Register(&gormigrate.Migration{
		ID:      "20260101000001",
		Migrate: func(tx *gorm.DB) error { return nil },
	})
}

// --- All (ordering) ---

func TestAll_ReturnsMigrationsInIDOrder(t *testing.T) {
	defer resetRegistry()()

	noop := func(tx *gorm.DB) error { return nil }
	Register(&gormigrate.Migration{ID: "20260101000003", Migrate: noop, Rollback: noop})
	Register(&gormigrate.Migration{ID: "20260101000001", Migrate: noop, Rollback: noop})
	Register(&gormigrate.Migration{ID: "20260101000002", Migrate: noop, Rollback: noop})

	got := All()
	wantOrder := []string{"20260101000001", "20260101000002", "20260101000003"}
	for i, m := range got {
		if m.ID != wantOrder[i] {
			t.Fatalf("position %d: expected %q, got %q", i, wantOrder[i], m.ID)
		}
	}
}

// --- Up / Down / RollbackTo ---

type testWidget struct {
	ID   uuid.UUID `gorm:"type:text;primaryKey"`
	Name string
}

func TestUp_AppliesAllMigrations(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	Register(&gormigrate.Migration{
		ID: "20260101000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Migrator().CreateTable(&testWidget{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&testWidget{})
		},
	})

	if err := Up(db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !db.Migrator().HasTable(&testWidget{}) {
		t.Fatal("expected test_widgets table to exist after Up")
	}

	// schema_migrations should record the applied migration.
	var count int64
	db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE id = ?", "20260101000001").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 schema_migrations row, got %d", count)
	}
}

func TestDown_RollsBackLatestMigration(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	noop := func(tx *gorm.DB) error { return nil }
	Register(&gormigrate.Migration{
		ID:      "20260101000001",
		Migrate: func(tx *gorm.DB) error { return tx.Migrator().CreateTable(&testWidget{}) },
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&testWidget{})
		},
	})
	Register(&gormigrate.Migration{
		ID:       "20260101000002",
		Migrate:  noop,
		Rollback: noop,
	})

	if err := Up(db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := Down(db); err != nil {
		t.Fatalf("Down: %v", err)
	}

	var count int64
	db.Raw("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 schema_migrations row after Down, got %d", count)
	}
}

func TestDown_ErrorsWhenNoMigrations(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	if err := Down(db); err == nil {
		t.Fatal("expected error when registry is empty")
	}
}

func TestRollbackTo_RollsBackToTarget(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	noop := func(tx *gorm.DB) error { return nil }
	Register(&gormigrate.Migration{ID: "20260101000001", Migrate: noop, Rollback: noop})
	Register(&gormigrate.Migration{ID: "20260101000002", Migrate: noop, Rollback: noop})
	Register(&gormigrate.Migration{ID: "20260101000003", Migrate: noop, Rollback: noop})

	if err := Up(db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := RollbackTo(db, "20260101000001"); err != nil {
		t.Fatalf("RollbackTo: %v", err)
	}

	var ids []string
	db.Raw("SELECT id FROM schema_migrations ORDER BY id").Scan(&ids)
	if len(ids) != 1 || ids[0] != "20260101000001" {
		t.Fatalf("expected [20260101000001], got %v", ids)
	}
}

// --- RegisterModel convenience ---

func TestRegisterModel_CreatesAndDropsTable(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	RegisterModel("20260101000001", &testWidget{})

	if err := Up(db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !db.Migrator().HasTable(&testWidget{}) {
		t.Fatal("expected table after Up")
	}

	if err := Down(db); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if db.Migrator().HasTable(&testWidget{}) {
		t.Fatal("expected table dropped after Down")
	}
}
