package job

import (
	"testing"

	"github.com/riverqueue/river"
)

func TestRiverInsertOpts(t *testing.T) {
	if got := riverInsertOpts(nil); got != nil {
		t.Fatalf("riverInsertOpts(nil) = %+v, want nil", got)
	}
	got := riverInsertOpts(&InsertOpts{Queue: "provisioning"})
	if got == nil {
		t.Fatal("riverInsertOpts(non-nil) = nil, want mapped opts")
	}
	if got.Queue != "provisioning" {
		t.Fatalf("riverInsertOpts Queue = %q, want %q", got.Queue, "provisioning")
	}
}

// DefaultQueue must track River's own default, since callers use it as the map
// key for the default pool and to leave InsertOpts.Queue empty.
func TestDefaultQueueMatchesRiver(t *testing.T) {
	if DefaultQueue != river.QueueDefault {
		t.Fatalf("DefaultQueue = %q, want river.QueueDefault %q", DefaultQueue, river.QueueDefault)
	}
}

// RunWorker must reject a nil registry before it ever touches the DB — a worker
// process with no workers registered is a programming error, not a runtime one.
func TestRunWorkerRequiresWorkers(t *testing.T) {
	if err := RunWorker(nil, Config{}); err == nil {
		t.Fatal("RunWorker(nil, ...) returned nil error, want a non-nil-registry error")
	}
}
