package job

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river/rivertype"
)

// logMiddleware only logs — the contract under test is that it invokes the
// inner work exactly once and passes its result through unchanged, on all
// three logging branches (success, will-retry, giving-up).

func loggedRow(attempt, maxAttempts int) *rivertype.JobRow {
	return &rivertype.JobRow{
		ID: 7, Kind: "probe", Attempt: attempt, MaxAttempts: maxAttempts,
		EncodedArgs: []byte(`{}`),
	}
}

func TestLogMiddleware_SuccessPassesThrough(t *testing.T) {
	calls := 0
	err := (&logMiddleware{}).Work(context.Background(), loggedRow(1, 3), func(context.Context) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want nil/1", err, calls)
	}
}

func TestLogMiddleware_RetryableFailureReturnsError(t *testing.T) {
	want := errors.New("transient")
	err := (&logMiddleware{}).Work(context.Background(), loggedRow(1, 3), func(context.Context) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
}

func TestLogMiddleware_FinalFailureReturnsError(t *testing.T) {
	want := errors.New("fatal")
	err := (&logMiddleware{}).Work(context.Background(), loggedRow(3, 3), func(context.Context) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
}
