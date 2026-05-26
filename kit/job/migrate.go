package job

import (
	"context"
	"fmt"

	"github.com/mashbot-co/gocore/db/connection"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

// MigrateUp creates River's schema (the job-queue tables) on gocore's DB
// connection. Call it from a gormigrate migration's Migrate func; the app owns
// the migration ID (and thus ordering), gocore owns the River-specific logic.
//
// It runs River's own migrator on the connection pool — deliberately not
// inside the caller's transaction — because River's MigrateTx is deprecated:
// some River migrations can't share one transaction. The migrator is
// idempotent, so re-running after a partial failure is a no-op for versions
// already applied. Bumping the river dependency may add River migrations;
// re-running MigrateUp picks them up.
func MigrateUp(ctx context.Context) error { return migrate(ctx, rivermigrate.DirectionUp) }

// MigrateDown rolls River's schema back. Pair it with MigrateUp in a
// migration's Rollback func.
func MigrateDown(ctx context.Context) error { return migrate(ctx, rivermigrate.DirectionDown) }

func migrate(ctx context.Context, direction rivermigrate.Direction) error {
	pool, err := connection.DB().DB()
	if err != nil {
		return fmt.Errorf("job migrate: get *sql.DB: %w", err)
	}
	migrator, err := rivermigrate.New(riverdatabasesql.New(pool), nil)
	if err != nil {
		return fmt.Errorf("job migrate: new migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, direction, nil); err != nil {
		return fmt.Errorf("job migrate %s: %w", direction, err)
	}
	return nil
}
