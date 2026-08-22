package middleware

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mashbot-co/gocore"
)

func TestSetScopeOnGORM_UnknownScopeLeavesContext(t *testing.T) {
	gocore.Reset()
	gocore.Init(gocore.Config{ScopeName: "warehouse"}) // no GORM routing for this concept
	t.Cleanup(gocore.Reset)

	ctx := context.Background()
	if got := setScopeOnGORM(ctx, uuid.New()); got != ctx {
		t.Error("an unrouted scope name must return the context unchanged")
	}
}

func TestCapitalizeFirst(t *testing.T) {
	cases := map[string]string{"": "", "project": "Project", "Tenant": "Tenant", "x": "X"}
	for in, want := range cases {
		if got := capitalizeFirst(in); got != want {
			t.Errorf("capitalizeFirst(%q) = %q, want %q", in, got, want)
		}
	}
}
