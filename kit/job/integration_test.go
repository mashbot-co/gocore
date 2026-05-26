package job

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/mashbot-co/gocore/db/connection"
	"github.com/riverqueue/river"
)

// These tests exercise the real River+Postgres paths (NewClient, Enqueue,
// EnqueueTx, Start/Stop, RunWorker, MigrateUp/Down). They follow gocore's
// convention: skipped under -short, and skipped (not failed) when Postgres
// isn't reachable, so `go test -short` stays green everywhere while the full
// suite (make check-coverage --full, CI with Postgres) covers them.

type probeArgs struct{}

func (probeArgs) Kind() string { return "job_probe" }

type probeWorker struct {
	river.WorkerDefaults[probeArgs]
}

func (probeWorker) Work(context.Context, *river.Job[probeArgs]) error { return nil }

func requirePostgres(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: skipping Postgres-dependent job test")
	}
	connection.Reset()
	if _, err := connection.Setup(); err != nil {
		t.Skipf("Postgres not reachable: %v", err)
	}
	t.Cleanup(connection.Reset)
}

func TestMigrate_Enqueue_StartStop(t *testing.T) {
	requirePostgres(t)
	ctx := context.Background()

	if err := MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	// Enqueue-only client (nil workers) — the API-process shape.
	producer, err := NewClient(nil, Config{})
	if err != nil {
		t.Fatalf("NewClient(enqueue-only): %v", err)
	}
	if err := producer.Enqueue(ctx, probeArgs{}, &InsertOpts{Queue: DefaultQueue}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Transactional enqueue on the underlying *sql.Tx.
	sqlDB, err := connection.DB().DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := producer.EnqueueTx(ctx, tx, probeArgs{}, nil); err != nil {
		t.Fatalf("EnqueueTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Worker client: start, let it drain the probe jobs, stop.
	workers := river.NewWorkers()
	river.AddWorker(workers, &probeWorker{})
	worker, err := NewClient(workers, Config{Queues: map[string]int{DefaultQueue: 2}})
	if err != nil {
		t.Fatalf("NewClient(worker): %v", err)
	}
	if err := worker.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := MigrateDown(ctx); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}
}

// TestNewClient_BadConfig drives the river.NewClient error branch: River
// rejects a queue with a non-positive worker count.
func TestNewClient_BadConfig(t *testing.T) {
	requirePostgres(t)
	if _, err := NewClient(river.NewWorkers(), Config{Queues: map[string]int{DefaultQueue: 0}}); err == nil {
		t.Fatal("expected error from NewClient with MaxWorkers=0")
	}
}

// TestRunWorker_NewClientError drives RunWorker's construction-failure return.
func TestRunWorker_NewClientError(t *testing.T) {
	requirePostgres(t)
	workers := river.NewWorkers()
	river.AddWorker(workers, &probeWorker{})
	if err := RunWorker(workers, Config{Queues: map[string]int{DefaultQueue: -1}}); err == nil {
		t.Fatal("expected RunWorker to propagate NewClient error")
	}
}

// TestMigrate_CanceledContext drives the migrator error branch.
func TestMigrate_CanceledContext(t *testing.T) {
	requirePostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := MigrateUp(ctx); err == nil {
		t.Fatal("expected MigrateUp error with a canceled context")
	}
}

// TestRunWorker covers the lifecycle wrapper: it starts, then a SIGTERM (caught
// by RunWorker's own signal context, so the process isn't killed) triggers the
// graceful drain-and-return path.
func TestRunWorker(t *testing.T) {
	requirePostgres(t)
	if err := MigrateUp(context.Background()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	t.Cleanup(func() { _ = MigrateDown(context.Background()) })

	workers := river.NewWorkers()
	river.AddWorker(workers, &probeWorker{})

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()

	if err := RunWorker(workers, Config{Queues: map[string]int{DefaultQueue: 1}}); err != nil {
		t.Fatalf("RunWorker: %v", err)
	}
}
