// Package migrate implements the orchestration logic for consumer migration
// CLIs and Lambda handlers. Consumers expose a thin shim that imports their
// own migrations package (for init() registration) and calls RunCLI or
// RunLambda from this package.
//
// Why this isn't a standalone binary
//
// Migrations register themselves with the migrations runner via init() side
// effects. For init() to fire, the consumer's migrations package must be
// linked into the running binary. gocore can't generically know the
// consumer's import path, so the consumer must own the binary's main()
// — even if it's a 5-line file that just blank-imports its migrations and
// calls into this package. See docs/architecture for the full discussion.
package migrate

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"github.com/mashbot-co/gocore/config"
	"github.com/mashbot-co/gocore/db/connection"
	"github.com/mashbot-co/gocore/db/migrations"
)

// RunCLI is the local-mode entry point. Called from the consumer's
// `tools/migrate/main.go` (build tag !lambda.norpc). Reads os.Args, opens
// the local Postgres via connection.Setup, and dispatches the subcommand.
//
// The real logic lives in runCLI below; RunCLI just wires up env-file
// loading, the global connection.Setup, and the os.Exit-on-error
// behavior that's hard to test in process.
func RunCLI() {
	if envFile := findEnvFile(); envFile != "" {
		godotenv.Load(envFile)
	}
	if err := runCLI(os.Args, connection.Setup); err != nil {
		log.Fatalf("%v", err)
	}
}

// runCLI is the inner, testable form of RunCLI. The setup parameter lets
// tests inject a fake without needing a real Postgres connection. Returns
// nil on success and a wrapped error on any failure (caller decides how
// to surface — RunCLI uses log.Fatalf).
func runCLI(args []string, setup func() (*gorm.DB, error)) error {
	if len(args) < 2 {
		usage()
		return fmt.Errorf("missing command")
	}
	db, err := setup()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	return dispatchCLI(db, args[1])
}

// dispatchCLI runs a single CLI subcommand against the given DB. Split out
// from RunCLI so tests can drive command dispatch without going through
// os.Args + connection.Setup.
func dispatchCLI(gormDB *gorm.DB, command string) error {
	switch command {
	case "up":
		if err := migrations.Up(gormDB); err != nil {
			return fmt.Errorf("migrate up failed: %w", err)
		}
	case "down":
		if err := migrations.Down(gormDB); err != nil {
			return fmt.Errorf("migrate down failed: %w", err)
		}
	case "reset":
		return resetDB(gormDB)
	case "tables":
		printTables(gormDB)
	case "status":
		printStatus(gormDB)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		usage()
		return fmt.Errorf("unknown command: %s", command)
	}
	return nil
}

// RunLambda is the Lambda-mode entry point. Called from the consumer's
// `tools/migrate/lambda.go` (build tag lambda.norpc). Hydrates config from
// SSM and starts the Lambda runtime with our event handler.
func RunLambda() {
	log.Println("migrate-lambda: starting")
	lambda.Start(handler)
}

// Request is the Lambda event shape: a JSON object with a "command" field.
type Request struct {
	Command string `json:"command"`
}

// Response is what we return to the Lambda invoker.
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func handler(ctx context.Context, req Request) (Response, error) {
	if err := config.LoadFromSSM(ctx); err != nil {
		return Response{Status: "error", Message: fmt.Sprintf("failed to load config: %v", err)}, err
	}

	gormDB, err := connection.Setup()
	if err != nil {
		return Response{Status: "error", Message: fmt.Sprintf("failed to connect: %v", err)}, err
	}

	return dispatchLambda(gormDB, req.Command)
}

// dispatchLambda runs a single Lambda invocation's command against the given
// DB. Split out from handler so tests can drive command dispatch without
// going through config.LoadFromSSM + connection.Setup.
func dispatchLambda(gormDB *gorm.DB, command string) (Response, error) {
	if command == "" {
		command = "up"
	}

	switch command {
	case "up":
		if err := migrations.Up(gormDB); err != nil {
			return Response{Status: "error", Message: fmt.Sprintf("migrate up failed: %v", err)}, err
		}
		return Response{Status: "ok", Message: "all migrations applied"}, nil
	case "down":
		if err := migrations.Down(gormDB); err != nil {
			return Response{Status: "error", Message: fmt.Sprintf("migrate down failed: %v", err)}, err
		}
		return Response{Status: "ok", Message: "rolled back last migration"}, nil
	default:
		return Response{Status: "error", Message: fmt.Sprintf("unknown command: %s", command)}, fmt.Errorf("unknown command: %s", command)
	}
}

// --- subcommand implementations ---

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: migrate <command>\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  up      Run all pending migrations\n")
	fmt.Fprintf(os.Stderr, "  down    Roll back the last migration\n")
	fmt.Fprintf(os.Stderr, "  reset   Drop all tables and re-run all migrations\n")
	fmt.Fprintf(os.Stderr, "  tables  List all tables in the database\n")
	fmt.Fprintf(os.Stderr, "  status  Show migration status\n")
}

func printStatus(gormDB *gorm.DB) {
	type record struct {
		ID string
	}

	if !gormDB.Migrator().HasTable("schema_migrations") {
		fmt.Println("No migrations have been run yet.")
		fmt.Println()
		for _, m := range migrations.All() {
			fmt.Printf("  %-20s pending\n", m.ID)
		}
		return
	}

	var applied []record
	gormDB.Raw("SELECT id FROM schema_migrations ORDER BY id").Scan(&applied)

	appliedSet := make(map[string]bool)
	for _, r := range applied {
		appliedSet[r.ID] = true
	}

	all := migrations.All()
	if len(all) == 0 {
		fmt.Println("No migrations registered.")
		return
	}

	fmt.Printf("%-20s %s\n", "MIGRATION", "STATUS")
	fmt.Printf("%-20s %s\n", "---------", "------")
	for _, m := range all {
		status := "pending"
		if appliedSet[m.ID] {
			status = "applied"
		}
		fmt.Printf("%-20s %s\n", m.ID, status)
	}
}

func printTables(gormDB *gorm.DB) {
	var tables []string
	gormDB.Raw(`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`).Scan(&tables)
	if len(tables) == 0 {
		fmt.Println("No tables found.")
		return
	}
	for _, t := range tables {
		fmt.Println(t)
	}
}

func resetDB(gormDB *gorm.DB) error {
	var tables []string
	gormDB.Raw(`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`).Scan(&tables)

	if len(tables) > 0 {
		for _, t := range tables {
			gormDB.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q CASCADE", t))
		}
		log.Printf("migrate: dropped %d tables\n", len(tables))
	}

	var types []string
	gormDB.Raw(`SELECT typname FROM pg_type WHERE typnamespace = 'public'::regnamespace AND typtype = 'e'`).Scan(&types)
	for _, t := range types {
		gormDB.Exec(fmt.Sprintf("DROP TYPE IF EXISTS %q CASCADE", t))
	}
	if len(types) > 0 {
		log.Printf("migrate: dropped %d enum types\n", len(types))
	}

	if err := migrations.Up(gormDB); err != nil {
		return fmt.Errorf("migrate up after reset failed: %w", err)
	}
	return nil
}

func findEnvFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
