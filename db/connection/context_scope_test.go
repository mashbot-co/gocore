package connection

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestWithCurrentProject_And_CurrentProject(t *testing.T) {
	id := uuid.New()
	ctx := WithCurrentProject(context.Background(), id)
	if got := CurrentProject(ctx); got != id {
		t.Errorf("CurrentProject = %v, want %v", got, id)
	}
}

func TestCurrentProject_NotSet(t *testing.T) {
	if got := CurrentProject(context.Background()); got != uuid.Nil {
		t.Errorf("CurrentProject (unset) = %v, want Nil", got)
	}
}

func TestWithoutProjectScope(t *testing.T) {
	ctx := WithCurrentProject(context.Background(), uuid.New())
	ctx = WithoutProjectScope(ctx)
	if got := CurrentProject(ctx); got != uuid.Nil {
		t.Errorf("CurrentProject after WithoutProjectScope = %v, want Nil", got)
	}
}

func TestWithoutTenantScope(t *testing.T) {
	ctx := WithCurrentTenant(context.Background(), uuid.New())
	ctx = WithoutTenantScope(ctx)
	if got := CurrentTenant(ctx); got != uuid.Nil {
		t.Errorf("CurrentTenant after WithoutTenantScope = %v, want Nil", got)
	}
}

func TestWithIsAdmin_And_IsAdmin(t *testing.T) {
	if IsAdmin(context.Background()) {
		t.Error("IsAdmin (unset) = true, want false")
	}
	// WithIsAdmin(false) is a no-op: the flag stays unset/false.
	if IsAdmin(WithIsAdmin(context.Background(), false)) {
		t.Error("IsAdmin after WithIsAdmin(false) = true, want false")
	}
	if !IsAdmin(WithIsAdmin(context.Background(), true)) {
		t.Error("IsAdmin after WithIsAdmin(true) = false, want true")
	}
}
