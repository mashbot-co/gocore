package job

import (
	"context"
	"sync"
	"testing"

	"github.com/mashbot-co/gocore/db/connection"
	"gorm.io/gorm"
)

type ambientArgs struct{}

func (ambientArgs) Kind() string { return "ambient_probe" }

func resetDefault() {
	defaultMu.Lock()
	defaultEnabled = false
	defaultOnce = sync.Once{}
	defaultClient = nil
	defaultErr = nil
	defaultMu.Unlock()
}

// When the default enqueuer isn't enabled, Enqueue/EnqueueTx are silent no-ops
// (the tests / dev-without-a-queue path) and never touch the DB.
func TestDefaultEnqueuer_DisabledIsNoop(t *testing.T) {
	resetDefault()
	if err := Enqueue(context.Background(), ambientArgs{}, nil); err != nil {
		t.Errorf("Enqueue while disabled = %v, want nil no-op", err)
	}
	// &gorm.DB{} is never dereferenced because the disabled check returns first.
	if err := EnqueueTx(context.Background(), &gorm.DB{}, ambientArgs{}, nil); err != nil {
		t.Errorf("EnqueueTx while disabled = %v, want nil no-op", err)
	}
}

// Enabled, the ambient helpers build a client lazily and enqueue — plain and
// transactionally (via a gorm tx whose ConnPool is a *sql.Tx).
func TestDefaultEnqueuer_Enabled(t *testing.T) {
	requirePostgres(t)
	resetDefault()
	t.Cleanup(resetDefault)

	if err := MigrateUp(context.Background()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	EnableDefaultEnqueuer()

	if err := Enqueue(context.Background(), ambientArgs{}, &InsertOpts{Queue: DefaultQueue}); err != nil {
		t.Fatalf("ambient Enqueue: %v", err)
	}
	err := connection.DB().Transaction(func(tx *gorm.DB) error {
		return EnqueueTx(context.Background(), tx, ambientArgs{}, nil)
	})
	if err != nil {
		t.Fatalf("ambient EnqueueTx: %v", err)
	}
}
