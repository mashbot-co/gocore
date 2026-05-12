package migrations

import (
	"fmt"
	"log"
	"regexp"
	"sort"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var (
	registry  []*gormigrate.Migration
	idPattern = regexp.MustCompile(`^\d{14}$`)
)

// Register adds a migration to the registry. Called from init() in each migration file.
// The migration ID must be a 14-digit timestamp (e.g. "20250101000001").
func Register(m *gormigrate.Migration) {
	if !idPattern.MatchString(m.ID) {
		panic(fmt.Sprintf("migrate: invalid migration ID %q — must be a 14-digit timestamp", m.ID))
	}
	if m.Migrate == nil || m.Rollback == nil {
		panic(fmt.Sprintf("migrate: migration %q must have both Migrate and Rollback functions", m.ID))
	}
	registry = append(registry, m)
}

// RegisterModel registers a migration that creates a table for the given GORM model.
func RegisterModel(id string, model interface{}) {
	Register(&gormigrate.Migration{
		ID: id,
		Migrate: func(tx *gorm.DB) error {
			return tx.Migrator().CreateTable(model)
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(model)
		},
	})
}

// Reset clears the migration registry. Intended for use in tests only —
// callers should save migrations.All() and re-Register what they want
// preserved after a Reset.
func Reset() {
	registry = nil
}

// All returns all registered migrations sorted by ID (timestamp order).
func All() []*gormigrate.Migration {
	sorted := make([]*gormigrate.Migration, len(registry))
	copy(sorted, registry)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

func newMigrate(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, &gormigrate.Options{
		TableName:                 "schema_migrations",
		IDColumnName:              "id",
		IDColumnSize:              190,
		UseTransaction:            true,
		ValidateUnknownMigrations: true,
	}, All())
}

// Up runs all pending migrations.
func Up(db *gorm.DB) error {
	m := newMigrate(db)
	if err := m.Migrate(); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	log.Println("migrate: all migrations applied")
	return nil
}

// Down rolls back the last applied migration.
func Down(db *gorm.DB) error {
	all := All()
	if len(all) == 0 {
		return fmt.Errorf("no migrations to roll back")
	}
	m := newMigrate(db)
	last := all[len(all)-1]
	if err := m.RollbackMigration(last); err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	log.Printf("migrate: rolled back %s\n", last.ID)
	return nil
}

// RollbackTo rolls back all migrations after the given ID.
func RollbackTo(db *gorm.DB, id string) error {
	m := newMigrate(db)
	if err := m.RollbackTo(id); err != nil {
		return fmt.Errorf("rollback to %s: %w", id, err)
	}
	log.Printf("migrate: rolled back to %s\n", id)
	return nil
}
