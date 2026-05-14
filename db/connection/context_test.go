package connection_test

import (
	"context"
	"testing"

	"github.com/mashbot-co/gocore/db/connection"

	"github.com/google/uuid"
)

func TestWithCurrentUser_And_CurrentUser(t *testing.T) {
	userID := uuid.New()
	ctx := connection.WithCurrentUser(context.Background(), userID)

	got := connection.CurrentUser(ctx)
	if got != userID {
		t.Fatalf("expected %s, got %s", userID, got)
	}
}

func TestCurrentUser_NotSet(t *testing.T) {
	got := connection.CurrentUser(context.Background())
	if got != uuid.Nil {
		t.Fatalf("expected uuid.Nil, got %s", got)
	}
}

func TestWithCurrentTenant_And_CurrentTenant(t *testing.T) {
	tenantID := uuid.New()
	ctx := connection.WithCurrentTenant(context.Background(), tenantID)

	got := connection.CurrentTenant(ctx)
	if got != tenantID {
		t.Fatalf("expected %s, got %s", tenantID, got)
	}
}

func TestCurrentTenant_NotSet(t *testing.T) {
	got := connection.CurrentTenant(context.Background())
	if got != uuid.Nil {
		t.Fatalf("expected uuid.Nil, got %s", got)
	}
}

func TestBothUserAndTenant(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()

	ctx := connection.WithCurrentUser(context.Background(), userID)
	ctx = connection.WithCurrentTenant(ctx, tenantID)

	gotUser := connection.CurrentUser(ctx)
	gotTenant := connection.CurrentTenant(ctx)

	if gotUser != userID {
		t.Fatalf("expected user %s, got %s", userID, gotUser)
	}
	if gotTenant != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, gotTenant)
	}
}

func TestContextChaining(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()

	// Chain: WithCurrentUser then WithCurrentTenant on the result
	ctx := connection.WithCurrentTenant(
		connection.WithCurrentUser(context.Background(), userID),
		tenantID,
	)

	gotUser := connection.CurrentUser(ctx)
	gotTenant := connection.CurrentTenant(ctx)

	if gotUser != userID {
		t.Fatalf("expected user %s, got %s", userID, gotUser)
	}
	if gotTenant != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, gotTenant)
	}
}

func TestWithCurrentUser_OverwritesPrevious(t *testing.T) {
	first := uuid.New()
	second := uuid.New()

	ctx := connection.WithCurrentUser(context.Background(), first)
	ctx = connection.WithCurrentUser(ctx, second)

	got := connection.CurrentUser(ctx)
	if got != second {
		t.Fatalf("expected overwritten user %s, got %s", second, got)
	}
}

func TestWithCurrentTenant_OverwritesPrevious(t *testing.T) {
	first := uuid.New()
	second := uuid.New()

	ctx := connection.WithCurrentTenant(context.Background(), first)
	ctx = connection.WithCurrentTenant(ctx, second)

	got := connection.CurrentTenant(ctx)
	if got != second {
		t.Fatalf("expected overwritten tenant %s, got %s", second, got)
	}
}
