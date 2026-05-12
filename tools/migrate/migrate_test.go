package migrate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mashbot-co/gocore/connection"
	"github.com/mashbot-co/gocore/migrations"
)

// --- shared helpers ---

// openSQLite gives us an in-memory DB for unit-testing command dispatch.
// Postgres-specific code paths (resetDB, printTables) skip on SQLite —
// see their tests for explicit guards.
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

// resetMigrationRegistry empties the gormigrate registry. Each test that
// touches the registry calls this at the top so neighbouring tests can't
// leak migrations in either direction.
func resetMigrationRegistry(t *testing.T) {
	t.Helper()
	migrations.Reset()
	t.Cleanup(migrations.Reset)
}

// registerNoop registers a single no-op migration so callers don't have to
// spell out the function literals every time.
func registerNoop(id string) {
	noop := func(tx *gorm.DB) error { return nil }
	migrations.Register(&gormigrate.Migration{
		ID:       id,
		Migrate:  noop,
		Rollback: noop,
	})
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// what was written. Used to assert on human-readable output from
// printStatus / printTables.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	fn()
	w.Close()
	<-done
	return buf.String()
}

// captureStderr does the same for stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	fn()
	w.Close()
	<-done
	return buf.String()
}

// --- findEnvFile ---

func TestFindEnvFile_FindsInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("X=1\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// On macOS /tmp resolves to /private/tmp via a symlink; compare the
	// resolved paths so the assertion isn't path-shape sensitive.
	got, _ := filepath.EvalSymlinks(findEnvFile())
	want, _ := filepath.EvalSymlinks(envPath)
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestFindEnvFile_WalksUpToFindFile(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, ".env")
	if err := os.WriteFile(envPath, []byte("X=1\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(deep); err != nil {
		t.Fatalf("chdir deep: %v", err)
	}

	got, _ := filepath.EvalSymlinks(findEnvFile())
	want, _ := filepath.EvalSymlinks(envPath)
	if got != want {
		t.Fatalf("expected to walk up to %s, got %s", want, got)
	}
}

// --- usage ---

func TestUsage_PrintsKnownCommands(t *testing.T) {
	out := captureStderr(t, usage)
	for _, cmd := range []string{"up", "down", "reset", "tables", "status"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("expected %q in usage output, got:\n%s", cmd, out)
		}
	}
}

// --- dispatchLambda ---

func TestDispatchLambda_EmptyCommandDefaultsToUp(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")

	resp, err := dispatchLambda(db, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok status, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "applied") {
		t.Errorf("expected 'applied' in message, got %q", resp.Message)
	}
}

func TestDispatchLambda_UpAppliesPendingMigrations(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")

	resp, err := dispatchLambda(db, "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %+v", resp)
	}
}

func TestDispatchLambda_DownRollsBackLatest(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")
	registerNoop("20260101000002")

	if _, err := dispatchLambda(db, "up"); err != nil {
		t.Fatalf("up: %v", err)
	}
	resp, err := dispatchLambda(db, "down")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected ok status, got %+v", resp)
	}
}

func TestDispatchLambda_DownErrorsOnEmptyRegistry(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)

	resp, err := dispatchLambda(db, "down")
	if err == nil {
		t.Fatal("expected error for down with no migrations registered")
	}
	if resp.Status != "error" {
		t.Errorf("expected error status, got %+v", resp)
	}
}

func TestDispatchLambda_UnknownCommand(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)

	resp, err := dispatchLambda(db, "nonsense")
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if resp.Status != "error" {
		t.Errorf("expected error status, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "unknown command") {
		t.Errorf("expected 'unknown command' in message, got %q", resp.Message)
	}
}

// --- dispatchCLI ---

func TestDispatchCLI_UpSucceeds(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")

	if err := dispatchCLI(db, "up"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDispatchCLI_DownSucceeds(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")
	registerNoop("20260101000002")

	if err := dispatchCLI(db, "up"); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := dispatchCLI(db, "down"); err != nil {
		t.Fatalf("down: %v", err)
	}
}

func TestDispatchCLI_UnknownCommandErrors(t *testing.T) {
	db := openSQLite(t)
	_ = captureStderr(t, func() {
		if err := dispatchCLI(db, "nonsense"); err == nil {
			t.Fatal("expected error for unknown command")
		} else if !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("expected wrapped 'unknown command' error, got %v", err)
		}
	})
}

func TestDispatchCLI_StatusPrintsRegistry(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")

	out := captureStdout(t, func() {
		if err := dispatchCLI(db, "status"); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if !strings.Contains(out, "20260101000001") {
		t.Errorf("expected migration ID in status output:\n%s", out)
	}
}

func TestDispatchCLI_TablesNoOpOnEmpty(t *testing.T) {
	db := openSQLite(t)

	// printTables runs a Postgres-only query (pg_tables). On SQLite the
	// query fails silently, tables stays empty, and the function prints
	// the "No tables found." branch. Useful for verifying the empty path.
	out := captureStdout(t, func() {
		if err := dispatchCLI(db, "tables"); err != nil {
			t.Fatalf("tables: %v", err)
		}
	})
	if !strings.Contains(out, "No tables found") {
		t.Errorf("expected 'No tables found' on SQLite, got:\n%s", out)
	}
}

// --- printStatus directly ---

func TestPrintStatus_NoRegisteredMigrations(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)

	out := captureStdout(t, func() { printStatus(db) })
	if !strings.Contains(out, "No migrations") {
		t.Errorf("expected 'No migrations' message, got:\n%s", out)
	}
}

func TestPrintStatus_ShowsAppliedAfterUp(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")
	registerNoop("20260101000002")

	if err := migrations.Up(db); err != nil {
		t.Fatalf("up: %v", err)
	}

	out := captureStdout(t, func() { printStatus(db) })
	if !strings.Contains(out, "applied") {
		t.Errorf("expected 'applied' in status, got:\n%s", out)
	}
	if !strings.Contains(out, "20260101000001") {
		t.Errorf("expected first migration ID in status, got:\n%s", out)
	}
	if !strings.Contains(out, "20260101000002") {
		t.Errorf("expected second migration ID in status, got:\n%s", out)
	}
}

func TestPrintStatus_ShowsPendingBeforeRun(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")

	// schema_migrations table doesn't exist yet — hits the "No migrations
	// have been run yet" branch which lists all registered as pending.
	out := captureStdout(t, func() { printStatus(db) })
	if !strings.Contains(out, "pending") {
		t.Errorf("expected 'pending' in status, got:\n%s", out)
	}
}

// --- dispatchCLI reset branch (works on SQLite — pg queries silently
//     return empty, then migrations.Up runs) ---

func TestDispatchCLI_ResetSucceedsOnSQLite(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")

	if err := dispatchCLI(db, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
}

// --- dispatchCLI error-wrap paths (failing migration produces a wrapped
//     error from Up/Down) ---

// registerFailing adds a migration whose Migrate/Rollback both return an
// explicit error — useful for exercising the error-wrap branches in
// dispatchCLI / dispatchLambda.
func registerFailing(id string) {
	fail := func(tx *gorm.DB) error { return fmt.Errorf("boom") }
	migrations.Register(&gormigrate.Migration{
		ID:       id,
		Migrate:  fail,
		Rollback: fail,
	})
}

func TestDispatchCLI_UpWrapsMigrationError(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerFailing("20260101000001")

	err := dispatchCLI(db, "up")
	if err == nil {
		t.Fatal("expected wrapped error from failing migration")
	}
	if !strings.Contains(err.Error(), "migrate up failed") {
		t.Errorf("expected 'migrate up failed' prefix, got %v", err)
	}
}

func TestDispatchCLI_DownWrapsMigrationError(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)

	// Register and apply a no-op so there's something to roll back, then
	// also register a failing one applied so its Rollback runs and errors.
	registerNoop("20260101000001")
	registerFailing("20260101000002")
	// Manually run a custom up — gormigrate's batch Up would abort on the
	// failing one. We need to mark the failing migration as "applied" in
	// schema_migrations so Down tries to roll it back. Use a direct INSERT.
	if err := migrations.Up(db); err != nil {
		// expected — failing migration errors out
		_ = err
	}
	// Manually mark both migrations applied so Down picks the failing one.
	db.AutoMigrate(&schemaMigration{})
	db.Exec("INSERT OR REPLACE INTO schema_migrations (id) VALUES (?)", "20260101000001")
	db.Exec("INSERT OR REPLACE INTO schema_migrations (id) VALUES (?)", "20260101000002")

	err := dispatchCLI(db, "down")
	if err == nil {
		t.Fatal("expected wrapped error from failing rollback")
	}
	if !strings.Contains(err.Error(), "migrate down failed") {
		t.Errorf("expected 'migrate down failed' prefix, got %v", err)
	}
}

// schemaMigration shadows gormigrate's internal table so we can insert
// rows for the failure-rollback test above.
type schemaMigration struct {
	ID string `gorm:"primaryKey"`
}

func (schemaMigration) TableName() string { return "schema_migrations" }

// --- printStatus mixed applied/pending state ---

func TestPrintStatus_MixedAppliedAndPending(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)

	// Apply the first migration, then register a second without running
	// it — printStatus should show 1 applied + 1 pending.
	registerNoop("20260101000001")
	if err := migrations.Up(db); err != nil {
		t.Fatalf("first up: %v", err)
	}
	registerNoop("20260101000002")

	out := captureStdout(t, func() { printStatus(db) })
	if !strings.Contains(out, "applied") {
		t.Errorf("expected 'applied' line, got:\n%s", out)
	}
	if !strings.Contains(out, "pending") {
		t.Errorf("expected 'pending' line, got:\n%s", out)
	}
}

// --- handler SSM-error path ---

func TestHandler_SSMErrorBubblesUp(t *testing.T) {
	// Pointing SSM_PREFIX at a real-looking path that AWS can't resolve
	// without credentials forces config.LoadFromSSM to error. The
	// handler's "failed to load config" branch then fires.
	t.Setenv("SSM_PREFIX", "/this/path/does/not/exist")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "us-east-1")

	resp, err := handler(context.Background(), Request{Command: "up"})
	if err == nil {
		t.Skip("handler did not error — likely AWS creds available in this environment")
	}
	if resp.Status != "error" {
		t.Errorf("expected error status, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "failed to load config") {
		t.Errorf("expected 'failed to load config' in message, got %q", resp.Message)
	}
}

// --- Postgres-gated tests (skip on -short or when DB unreachable) ---

// requirePostgres skips if a local Postgres isn't reachable on localhost:5432
// with postgres/postgres/app credentials. Mirrors the pattern in the
// connection package's tests, plus drops schema_migrations so each test
// starts from a known state — tests that record migrations would otherwise
// confuse gormigrate's "no orphan rows in code" check across runs.
func requirePostgres(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres-dependent test in -short mode")
	}
	conn, err := net.DialTimeout("tcp", "localhost:5432", 200*time.Millisecond)
	if err != nil {
		t.Skipf("skipping: Postgres not reachable on localhost:5432 (%v)", err)
	}
	conn.Close()

	connection.Reset()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "local")

	db, err := connection.Setup()
	if err != nil {
		t.Skipf("could not connect to Postgres: %v", err)
	}
	// Drop schema_migrations and any test-planted tables so this test
	// starts from a clean slate. We don't drop EVERY table because the
	// DB might be shared with other tooling.
	db.Exec("DROP TABLE IF EXISTS schema_migrations")
	db.Exec("DROP TABLE IF EXISTS migrate_test_planted_table")
	db.Exec("DROP TABLE IF EXISTS migrate_test_tmp_TestPrintTables_ListsPostgresTables")
}

func TestPrintTables_ListsPostgresTables(t *testing.T) {
	requirePostgres(t)

	db, err := connection.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Postgres folds unquoted table names to lowercase, so use one already
	// lowercase to avoid case-sensitivity surprises in the assertion.
	tableName := "migrate_test_tmp_print_tables"
	db.Exec(`DROP TABLE IF EXISTS ` + tableName)
	if err := db.Exec(`CREATE TABLE ` + tableName + ` (id INT)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DROP TABLE IF EXISTS ` + tableName) })

	out := captureStdout(t, func() { printTables(db) })
	if !strings.Contains(out, tableName) {
		t.Errorf("expected %s in printTables output:\n%s", tableName, out)
	}
}

func TestResetDB_DropsTablesAndReapplies(t *testing.T) {
	requirePostgres(t)
	resetMigrationRegistry(t)

	db, err := connection.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Plant both a table AND an enum type so resetDB hits both the
	// "dropped N tables" and "dropped N enum types" log branches.
	planted := "migrate_test_planted_table"
	plantedEnum := "migrate_test_planted_enum"
	db.Exec(`DROP TABLE IF EXISTS ` + planted)
	db.Exec(`DROP TYPE IF EXISTS ` + plantedEnum)
	if err := db.Exec(`CREATE TABLE ` + planted + ` (id INT)`).Error; err != nil {
		t.Fatalf("create planted: %v", err)
	}
	if err := db.Exec(`CREATE TYPE ` + plantedEnum + ` AS ENUM ('a', 'b')`).Error; err != nil {
		t.Fatalf("create planted enum: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DROP TABLE IF EXISTS ` + planted)
		db.Exec(`DROP TYPE IF EXISTS ` + plantedEnum)
	})

	registerNoop("20260101000099")

	if err := resetDB(db); err != nil {
		t.Fatalf("resetDB: %v", err)
	}

	var tableCount int64
	db.Raw(`SELECT COUNT(*) FROM pg_tables WHERE tablename = ?`, planted).Scan(&tableCount)
	if tableCount != 0 {
		t.Fatalf("expected planted table dropped, count=%d", tableCount)
	}
	var enumCount int64
	db.Raw(`SELECT COUNT(*) FROM pg_type WHERE typname = ?`, plantedEnum).Scan(&enumCount)
	if enumCount != 0 {
		t.Fatalf("expected planted enum dropped, count=%d", enumCount)
	}
}

func TestResetDB_WrapsUpFailure(t *testing.T) {
	requirePostgres(t)
	resetMigrationRegistry(t)
	registerFailing("20260101000098")

	db, err := connection.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = resetDB(db)
	if err == nil {
		t.Fatal("expected wrapped error from failing migration during reset")
	}
	if !strings.Contains(err.Error(), "migrate up after reset failed") {
		t.Errorf("expected wrap message, got %v", err)
	}
}

func TestHandler_HappyPath(t *testing.T) {
	requirePostgres(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000100")

	resp, err := handler(context.Background(), Request{Command: "up"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %+v", resp)
	}
}

// --- runCLI inner function (the testable form of RunCLI) ---

func TestRunCLI_MissingCommandReturnsError(t *testing.T) {
	setup := func() (*gorm.DB, error) {
		t.Fatal("setup should not be called when args are missing")
		return nil, nil
	}
	_ = captureStderr(t, func() {
		err := runCLI([]string{"migrate"}, setup)
		if err == nil {
			t.Fatal("expected error for missing command")
		}
		if !strings.Contains(err.Error(), "missing command") {
			t.Errorf("expected 'missing command' error, got %v", err)
		}
	})
}

func TestRunCLI_SetupFailureBubblesUp(t *testing.T) {
	setup := func() (*gorm.DB, error) {
		return nil, fmt.Errorf("connection refused")
	}
	err := runCLI([]string{"migrate", "status"}, setup)
	if err == nil {
		t.Fatal("expected wrapped error from failing setup")
	}
	if !strings.Contains(err.Error(), "failed to connect to database") {
		t.Errorf("expected 'failed to connect to database' wrap, got %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected inner cause preserved, got %v", err)
	}
}

func TestRunCLI_HappyPathDispatchesCommand(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerNoop("20260101000001")

	setup := func() (*gorm.DB, error) { return db, nil }
	if err := runCLI([]string{"migrate", "up"}, setup); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunCLI_DispatchErrorBubblesUp(t *testing.T) {
	db := openSQLite(t)
	resetMigrationRegistry(t)
	registerFailing("20260101000001")

	setup := func() (*gorm.DB, error) { return db, nil }
	err := runCLI([]string{"migrate", "up"}, setup)
	if err == nil {
		t.Fatal("expected error from failing migration to bubble up")
	}
}

// RunLambda calls lambda.Start, which exits the process when invoked
// outside an AWS Lambda runtime. The subprocess pattern lets us exercise
// the function body (the log.Println + lambda.Start call) without trying
// to keep the parent test alive.
func TestRunLambda_StartsAndExits(t *testing.T) {
	if os.Getenv("MIGRATE_LAMBDA_SUBPROCESS") == "1" {
		RunLambda() // exits when no Lambda runtime is present
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRunLambda_StartsAndExits", "-test.timeout=5s")
	cmd.Env = append(os.Environ(), "MIGRATE_LAMBDA_SUBPROCESS=1")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "migrate-lambda: starting") {
		t.Fatalf("expected RunLambda to log startup, got:\n%s", out)
	}
}

// RunCLI happy path — re-exec subprocess with a real command. Requires
// Postgres so connection.Setup can succeed.
func TestRunCLI_HappyPath_RequiresPostgres(t *testing.T) {
	if os.Getenv("MIGRATE_CLI_HAPPY_SUBPROCESS") == "1" {
		// Subprocess: set env vars to point at Postgres + run "status".
		os.Setenv("DB_HOST", "localhost")
		os.Setenv("DB_PORT", "5432")
		os.Setenv("DB_NAME", "app")
		os.Setenv("DB_SECRET_ARN", "local")
		os.Args = []string{"migrate", "status"}
		// Drop schema_migrations so status hits the "No migrations have
		// been run yet" branch without confusing gormigrate.
		RunCLI()
		return
	}
	requirePostgres(t)
	cmd := exec.Command(os.Args[0], "-test.run=TestRunCLI_HappyPath_RequiresPostgres", "-test.timeout=10s")
	cmd.Env = append(os.Environ(), "MIGRATE_CLI_HAPPY_SUBPROCESS=1")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "No migrations") && !strings.Contains(string(out), "MIGRATION") {
		t.Fatalf("expected status output from RunCLI, got:\n%s", out)
	}
}
