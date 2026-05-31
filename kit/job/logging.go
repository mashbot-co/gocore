package job

import (
	"context"
	"log"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// logMiddleware logs every job's lifecycle — started, finished (with duration),
// or failed (distinguishing a will-retry from a final give-up) — so operators
// get a uniform trail across all handlers without each one logging by hand. It's
// registered on every worker Client in NewClient. The job's encoded args are
// included on the start line so the entity (e.g. project/app id) is visible.
type logMiddleware struct {
	river.MiddlewareDefaults
}

func (*logMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	start := time.Now()
	log.Printf("job %s started (id=%d attempt=%d/%d) args=%s",
		job.Kind, job.ID, job.Attempt, job.MaxAttempts, job.EncodedArgs)

	err := doInner(ctx)
	dur := time.Since(start).Round(time.Millisecond)

	switch {
	case err == nil:
		log.Printf("job %s finished (id=%d) in %s", job.Kind, job.ID, dur)
	case job.Attempt < job.MaxAttempts:
		log.Printf("job %s failed, will retry (id=%d attempt=%d/%d) after %s: %v",
			job.Kind, job.ID, job.Attempt, job.MaxAttempts, dur, err)
	default:
		log.Printf("job %s failed, giving up after %d attempt(s) (id=%d) in %s: %v",
			job.Kind, job.MaxAttempts, job.ID, dur, err)
	}
	return err
}
