package job

import (
	"context"
	"database/sql"
	"sync"

	"gorm.io/gorm"
)

// The process-wide default enqueuer powers the package-level Enqueue/EnqueueTx
// helpers, so producers (model hooks, resolvers) can emit jobs without each
// wiring their own client. A producer process opts in once at boot via
// EnableDefaultEnqueuer; until then the helpers are no-ops, which is the
// tests / dev-without-a-queue path.
var (
	defaultMu      sync.Mutex
	defaultEnabled bool
	defaultOnce    sync.Once
	defaultClient  *Client
	defaultErr     error
)

// EnableDefaultEnqueuer turns on the package-level Enqueue/EnqueueTx helpers.
// The DB need not be ready when this is called — the client is built lazily on
// first enqueue (by which point a request has a live connection).
func EnableDefaultEnqueuer() {
	defaultMu.Lock()
	defaultEnabled = true
	defaultMu.Unlock()
}

func ensureDefault() (*Client, error) {
	defaultMu.Lock()
	enabled := defaultEnabled
	defaultMu.Unlock()
	if !enabled {
		return nil, nil
	}
	defaultOnce.Do(func() { defaultClient, defaultErr = NewClient(nil, Config{}) })
	return defaultClient, defaultErr
}

// Enqueue inserts a job through the process default enqueuer. It's a no-op
// (nil error) when the default enqueuer hasn't been enabled.
func Enqueue(ctx context.Context, args Args, opts *InsertOpts) error {
	c, err := ensureDefault()
	if err != nil || c == nil {
		return err
	}
	return c.Enqueue(ctx, args, opts)
}

// EnqueueTx inserts a job on the given GORM transaction through the process
// default enqueuer, so the job becomes visible only once tx commits. If tx
// isn't an active SQL transaction it falls back to a plain enqueue. No-op when
// the default enqueuer hasn't been enabled.
func EnqueueTx(ctx context.Context, tx *gorm.DB, args Args, opts *InsertOpts) error {
	c, err := ensureDefault()
	if err != nil || c == nil {
		return err
	}
	if sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx); ok {
		return c.EnqueueTx(ctx, sqlTx, args, opts)
	}
	return c.Enqueue(ctx, args, opts)
}
