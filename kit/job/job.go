// Package job is gocore's background-job queue. It lives under kit/ — gocore's
// family of built-in framework capabilities (alongside kit/mailer) — and
// pointedly NOT under db/: a job queue is a capability, not a database concern,
// and the import path is part of the contract callers see. River (on the
// shared Postgres connection) is today's backend, but tomorrow's may be SQS or
// SNS, which are not databases at all — so the package's identity stays
// backend-neutral even though this implementation happens to be Postgres-backed.
//
// The boundary is deliberate. The PRODUCE side is abstracted behind the
// Enqueuer interface (plus the neutral Args/InsertOpts types), so app code that
// enqueues work depends only on this neutral surface — no River, no SQL, no
// "db" in the import — and the engine can be swapped with no impact to those
// callers. The CONSUME side is not abstracted: handlers are defined with
// River's typed workers directly, because River's value (typed jobs, retries,
// uniqueness, periodic scheduling, leader election, transactional enqueue) does
// not generalize across backends, and wrapping it would only strip those
// features. RunWorker owns the worker process lifecycle, the analog of
// server.Run for an API.
//
// When a second backend actually arrives, the River-specific pieces (Client,
// RunWorker, Migrate*) split into a kit/job/river adapter sub-package and the
// neutral types stay here — but that split waits until there is a real second
// consumer to justify it.
package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/mashbot-co/gocore/db/connection"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
)

// DefaultQueue is the queue name used when an enqueue specifies none.
const DefaultQueue = river.QueueDefault

// Args is the contract a job payload implements: a stable Kind() that routes it
// to its handler. It mirrors River's job-args interface, so the same struct
// satisfies both — produce-side callers depend only on this type.
type Args interface {
	Kind() string
}

// InsertOpts are backend-neutral enqueue options, kept intentionally minimal;
// extend as real needs appear rather than mirroring the engine wholesale.
type InsertOpts struct {
	// Queue routes the job to a named worker pool. Empty means DefaultQueue.
	Queue string
	// RunAt schedules the job to become available at a future time rather than
	// immediately — used for self-pacing reconcilers and retries. Zero value
	// means "run as soon as a worker is free".
	RunAt time.Time
}

// Enqueuer is the produce-side seam. App code (resolvers, model hooks) depends
// on this — never on River — so the backend can be swapped without touching
// call sites. *Client is the River-backed implementation.
//
// Note what's deliberately absent: transactional enqueue. Inserting a job
// inside a *sql.Tx is a Postgres-backed-queue feature (it's River's headline
// capability) and is meaningless for SQS/SNS, so it can't live in a neutral
// interface. It's exposed as the concrete Client.EnqueueTx instead — a caller
// that needs atomic enqueue is knowingly coupling to a DB-backed engine.
type Enqueuer interface {
	// Enqueue inserts a job to run as soon as a worker is free.
	Enqueue(ctx context.Context, args Args, opts *InsertOpts) error
}

// Config configures a job Client.
type Config struct {
	// Queues maps a queue name to its maximum concurrent workers. Required for
	// a worker (consuming) client; ignored for an enqueue-only client. Use
	// DefaultQueue as the key for the default pool.
	Queues map[string]int
}

// Client wraps a River client. It is both an Enqueuer (produce) and, when
// constructed with a workers registry, a worker runtime (consume).
type Client struct {
	river *river.Client[*sql.Tx]
}

// Compile-time check that *Client satisfies the produce-side seam.
var _ Enqueuer = (*Client)(nil)

// NewClient builds a job Client on gocore's DB connection — connection.Setup
// must have run first. Pass a non-nil workers registry for a worker process;
// pass nil for an enqueue-only client (e.g. an API server that only produces).
func NewClient(workers *river.Workers, cfg Config) (*Client, error) {
	pool, err := connection.DB().DB()
	if err != nil {
		return nil, fmt.Errorf("job: get *sql.DB: %w", err)
	}
	riverCfg := &river.Config{
		Workers: workers,
		// Uniform per-job lifecycle logging (started / finished / retry / give-up)
		// across every handler — see logMiddleware. Harmless on enqueue-only
		// clients (it only fires when a job is worked).
		Middleware: []rivertype.Middleware{&logMiddleware{}},
	}
	if len(cfg.Queues) > 0 {
		riverCfg.Queues = make(map[string]river.QueueConfig, len(cfg.Queues))
		for name, maxWorkers := range cfg.Queues {
			riverCfg.Queues[name] = river.QueueConfig{MaxWorkers: maxWorkers}
		}
	}
	rc, err := river.NewClient(riverdatabasesql.New(pool), riverCfg)
	if err != nil {
		return nil, fmt.Errorf("job: new river client: %w", err)
	}
	return &Client{river: rc}, nil
}

// Enqueue inserts a job to run as soon as a worker is free.
func (c *Client) Enqueue(ctx context.Context, args Args, opts *InsertOpts) error {
	_, err := c.river.Insert(ctx, args, riverInsertOpts(opts))
	return err
}

// EnqueueTx inserts a job within tx — visible to workers only once tx commits.
func (c *Client) EnqueueTx(ctx context.Context, tx *sql.Tx, args Args, opts *InsertOpts) error {
	_, err := c.river.InsertTx(ctx, tx, args, riverInsertOpts(opts))
	return err
}

func riverInsertOpts(opts *InsertOpts) *river.InsertOpts {
	if opts == nil {
		return nil
	}
	return &river.InsertOpts{Queue: opts.Queue, ScheduledAt: opts.RunAt}
}

// Start begins polling for and running jobs. Use on a worker client.
func (c *Client) Start(ctx context.Context) error { return c.river.Start(ctx) }

// Stop drains in-flight jobs and stops polling.
func (c *Client) Stop(ctx context.Context) error { return c.river.Stop(ctx) }

// RunWorker builds a worker Client from the registry, starts it, and blocks
// until SIGINT/SIGTERM, then drains in-flight jobs. It is the worker-process
// analog of server.Run — a single call that owns the lifecycle.
func RunWorker(workers *river.Workers, cfg Config) error {
	if workers == nil {
		return errors.New("job: RunWorker requires a non-nil workers registry")
	}
	client, err := NewClient(workers, cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("job: start: %w", err)
	}
	log.Printf("job: worker started — polling for jobs")

	<-ctx.Done()
	log.Printf("job: shutdown signal received, draining in-flight jobs")

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		return fmt.Errorf("job: graceful stop: %w", err)
	}
	log.Printf("job: worker stopped")
	return nil
}
