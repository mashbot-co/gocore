package migrations

import (
	"bytes"
	"errors"
	"fmt"
	"log"
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

// TestReset covers the exported Reset helper (added for test harnesses
// that need a clean global registry across runs).
func TestReset_ClearsRegistry(t *testing.T) {
	defer resetRegistry()()

	noop := func(tx *gorm.DB) error { return nil }
	Register(&gormigrate.Migration{ID: "20260101000001", Migrate: noop, Rollback: noop})
	if len(registry) != 1 {
		t.Fatalf("expected 1 registered migration before Reset, got %d", len(registry))
	}

	Reset()
	if len(registry) != 0 {
		t.Fatalf("expected empty registry after Reset, got %d entries", len(registry))
	}
}

// Up/Down/RollbackTo each have an error-wrap branch that fires when the
// underlying gormigrate call fails. The cleanest way to trigger that is a
// migration that returns an error from its Migrate/Rollback function.
func TestUp_WrapsMigrationError(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	fail := func(tx *gorm.DB) error { return errBoom() }
	Register(&gormigrate.Migration{ID: "20260101000001", Migrate: fail, Rollback: fail})

	err := Up(db)
	if err == nil {
		t.Fatal("expected wrapped error from failing migration")
	}
	if got := err.Error(); !contains(got, "migrate up") {
		t.Errorf("expected 'migrate up' prefix in error, got %q", got)
	}
}

func TestDown_WrapsRollbackError(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	// First register a no-op so there's something to apply.
	noop := func(tx *gorm.DB) error { return nil }
	Register(&gormigrate.Migration{ID: "20260101000001", Migrate: noop, Rollback: noop})
	if err := Up(db); err != nil {
		t.Fatalf("up: %v", err)
	}

	// Swap in a failing rollback by re-registering with the same ID and
	// directly inserting into schema_migrations (skipping past validation).
	registry = nil
	Register(&gormigrate.Migration{
		ID:       "20260101000001",
		Migrate:  noop,
		Rollback: func(tx *gorm.DB) error { return errBoom() },
	})

	err := Down(db)
	if err == nil {
		t.Fatal("expected wrapped error from failing rollback")
	}
	if got := err.Error(); !contains(got, "migrate down") {
		t.Errorf("expected 'migrate down' prefix in error, got %q", got)
	}
}

func TestRollbackTo_WrapsError(t *testing.T) {
	defer resetRegistry()()
	db := openSQLite(t)

	// RollbackTo a nonexistent target should error.
	err := RollbackTo(db, "99999999999999")
	if err == nil {
		t.Fatal("expected error rolling back to nonexistent target")
	}
	if got := err.Error(); !contains(got, "rollback to") {
		t.Errorf("expected 'rollback to' prefix in error, got %q", got)
	}
}

// --- tiny local helpers (kept here so the test file is self-contained) ---

func errBoom() error { return &boomError{} }

type boomError struct{}

func (b *boomError) Error() string { return "boom" }

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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

// --- Pending / Up logging ---

// captureLog redirects the standard logger for the duration of a test, so the
// assertions are about what an operator actually sees during a deploy.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })
	return &buf
}

func TestPending_ReturnsEverythingWhenNothingApplied(t *testing.T) {
	defer resetRegistry()()
	Register(&gormigrate.Migration{ID: "20260101000001",
		Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})
	Register(&gormigrate.Migration{ID: "20260101000002",
		Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})

	pending, err := Pending(openSQLite(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("Pending() = %d migrations, want 2", len(pending))
	}
}

// The point of reading schema_migrations rather than inferring: a migration
// applied out of band must not be reported as still pending.
func TestPending_ExcludesAlreadyApplied(t *testing.T) {
	defer resetRegistry()()
	Register(&gormigrate.Migration{ID: "20260101000001",
		Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})
	Register(&gormigrate.Migration{ID: "20260101000002",
		Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})

	db := openSQLite(t)
	if err := Up(db); err != nil {
		t.Fatalf("first Up: %v", err)
	}

	pending, err := Pending(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("Pending() after Up = %d, want 0", len(pending))
	}
}

func TestUp_LogsCountThenEachMigrationWithTiming(t *testing.T) {
	defer resetRegistry()()
	for _, id := range []string{"20260101000001", "20260101000002", "20260101000003"} {
		Register(&gormigrate.Migration{ID: id,
			Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})
	}

	out := captureLog(t)
	if err := Up(openSQLite(t)); err != nil {
		t.Fatalf("Up: %v", err)
	}
	got := out.String()

	for _, want := range []string{
		"3 pending migration(s) to apply",
		"[1/3] 20260101000001 starting",
		"[1/3] 20260101000001 done in",
		"[2/3] 20260101000002 starting",
		"[3/3] 20260101000003 done in",
		"applied 3 migration(s) in",
	} {
		if !contains(got, want) {
			t.Errorf("log missing %q\ngot:\n%s", want, got)
		}
	}
}

// A second run must say so plainly rather than logging a count of zero and a
// duration, which reads like work happened.
func TestUp_SaysNothingToDoWhenAlreadyCurrent(t *testing.T) {
	defer resetRegistry()()
	Register(&gormigrate.Migration{ID: "20260101000001",
		Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})

	db := openSQLite(t)
	if err := Up(db); err != nil {
		t.Fatalf("first Up: %v", err)
	}

	out := captureLog(t)
	if err := Up(db); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	got := out.String()
	if !contains(got, "no pending migrations") {
		t.Errorf("want 'no pending migrations', got:\n%s", got)
	}
	if contains(got, "starting") {
		t.Errorf("second run logged a migration starting:\n%s", got)
	}
}

// A failure has to name WHICH migration failed and how long it ran; a bare
// wrapped error leaves an operator diffing the registry against the table.
func TestUp_LogsWhichMigrationFailed(t *testing.T) {
	defer resetRegistry()()
	Register(&gormigrate.Migration{ID: "20260101000001",
		Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})
	Register(&gormigrate.Migration{ID: "20260101000002",
		Migrate: func(tx *gorm.DB) error { return errors.New("boom") }, Rollback: func(tx *gorm.DB) error { return nil }})

	out := captureLog(t)
	if err := Up(openSQLite(t)); err == nil {
		t.Fatal("expected Up to fail")
	}
	got := out.String()
	if !contains(got, "[2/2] 20260101000002 FAILED after") {
		t.Errorf("log should name the failing migration, got:\n%s", got)
	}
}

// The registry hands out pointers; wrapping must not mutate them, or a second
// Up in the same process would log through two layers of wrapper.
func TestUp_DoesNotMutateTheRegistry(t *testing.T) {
	defer resetRegistry()()
	Register(&gormigrate.Migration{ID: "20260101000001",
		Migrate: func(tx *gorm.DB) error { return nil }, Rollback: func(tx *gorm.DB) error { return nil }})

	before := All()[0].Migrate
	if err := Up(openSQLite(t)); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if fmt.Sprintf("%p", All()[0].Migrate) != fmt.Sprintf("%p", before) {
		t.Error("Up replaced the registered migration's Migrate function")
	}
}
