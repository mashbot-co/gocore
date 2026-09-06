package migrations

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"time"

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

// options are shared by every runner so the table name and transaction
// behaviour cannot drift between Up and the rollback paths.
func options() *gormigrate.Options {
	return &gormigrate.Options{
		TableName:                 "schema_migrations",
		IDColumnName:              "id",
		IDColumnSize:              190,
		UseTransaction:            true,
		ValidateUnknownMigrations: true,
	}
}

func newMigrate(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, options(), All())
}

// Pending returns the registered migrations that have not been applied yet,
// in the order they will run.
//
// Read from schema_migrations rather than inferred: a migration can be
// applied out of band, and the deploy needs to report what it is ABOUT to do
// rather than what it assumes is left.
func Pending(db *gorm.DB) ([]*gormigrate.Migration, error) {
	all := All()
	if !db.Migrator().HasTable("schema_migrations") {
		return all, nil
	}

	var ids []string
	if err := db.Raw("SELECT id FROM schema_migrations").Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("reading schema_migrations: %w", err)
	}
	applied := make(map[string]bool, len(ids))
	for _, id := range ids {
		applied[id] = true
	}

	pending := make([]*gormigrate.Migration, 0, len(all))
	for _, m := range all {
		if !applied[m.ID] {
			pending = append(pending, m)
		}
	}
	return pending, nil
}

// instrumented returns COPIES of the migrations whose Migrate functions log
// when they start, when they finish, and how long they took.
//
// Copies, because All() hands out pointers into the registry and tests reuse
// them -- mutating those would make a migration log twice on a second run in
// the same process, and would leave the registry permanently wrapped.
//
// Wrapping rather than driving each migration with MigrateTo: gormigrate
// decides transaction boundaries and ordering, and stepping it manually to
// get timing would replace behaviour that works with behaviour that merely
// looks the same.
func instrumented(all []*gormigrate.Migration, order map[string]int, total int) []*gormigrate.Migration {
	out := make([]*gormigrate.Migration, 0, len(all))
	for _, m := range all {
		m := m
		n, isPending := order[m.ID]
		if !isPending {
			out = append(out, m)
			continue
		}
		migrate := m.Migrate
		wrapped := *m
		wrapped.Migrate = func(tx *gorm.DB) error {
			log.Printf("migrate: [%d/%d] %s starting", n, total, m.ID)
			start := time.Now()
			if err := migrate(tx); err != nil {
				log.Printf("migrate: [%d/%d] %s FAILED after %s: %v",
					n, total, m.ID, time.Since(start).Round(time.Millisecond), err)
				return err
			}
			log.Printf("migrate: [%d/%d] %s done in %s",
				n, total, m.ID, time.Since(start).Round(time.Millisecond))
			return nil
		}
		out = append(out, &wrapped)
	}
	return out
}

// Up runs all pending migrations, reporting how many there are before it
// starts and timing each one.
//
// The count comes first because that is the number an operator needs while
// deciding whether to wait: a deploy blocks on this, and "17 to apply" and
// "1 to apply" are very different waits. Per-migration timing then shows
// WHICH one is slow, rather than leaving a single long silence to interpret.
func Up(db *gorm.DB) error {
	pending, err := Pending(db)
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	if len(pending) == 0 {
		log.Println("migrate: no pending migrations")
		return nil
	}

	order := make(map[string]int, len(pending))
	for i, m := range pending {
		order[m.ID] = i + 1
	}
	log.Printf("migrate: %d pending migration(s) to apply", len(pending))

	m := gormigrate.New(db, options(), instrumented(All(), order, len(pending)))
	start := time.Now()
	if err := m.Migrate(); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	log.Printf("migrate: applied %d migration(s) in %s",
		len(pending), time.Since(start).Round(time.Millisecond))
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
