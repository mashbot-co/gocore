package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore"
	"github.com/mashbot-co/gocore/db/connection"
)

// seenProject runs ScopeContext (behind an optional pre-middleware) and reports
// the project id the final handler sees on the request's GORM context.
func seenProject(pre gin.HandlerFunc, req *http.Request) uuid.UUID {
	r := gin.New()
	if pre != nil {
		r.Use(pre)
	}
	r.Use(ScopeContext())
	var seen uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		seen = connection.CurrentProject(c.Request.Context())
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

func TestScopeContext_FromClaim(t *testing.T) {
	gocore.Init(gocore.Config{ScopeName: "project"})
	t.Cleanup(gocore.Reset)
	id := uuid.New()

	pre := func(c *gin.Context) { c.Set("project_id", id) } // simulates JWTAuth's lift
	if got := seenProject(pre, httptest.NewRequest(http.MethodGet, "/x", nil)); got != id {
		t.Fatalf("claim path: got %v, want %v", got, id)
	}
}

func TestScopeContext_FromStubHeader(t *testing.T) {
	gocore.Init(gocore.Config{ScopeName: "project"}) // default prefix X-Stub-
	t.Cleanup(gocore.Reset)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Stub-Project-Id", id.String())
	if got := seenProject(nil, req); got != id {
		t.Fatalf("stub-header path: got %v, want %v", got, id)
	}
}

func TestScopeContext_NoScopeConfigured(t *testing.T) {
	gocore.Reset() // ScopeName empty → scopeIDFromRequest returns Nil
	if got := seenProject(nil, httptest.NewRequest(http.MethodGet, "/x", nil)); got != uuid.Nil {
		t.Fatalf("no scope: got %v, want Nil", got)
	}
}

// TestScopeContext_TenantSetter covers setScopeOnGORM's "tenant" branch.
func TestScopeContext_TenantSetter(t *testing.T) {
	gocore.Init(gocore.Config{ScopeName: "tenant"})
	t.Cleanup(gocore.Reset)
	id := uuid.New()

	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", id) })
	r.Use(ScopeContext())
	var seen uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		seen = connection.CurrentTenant(c.Request.Context())
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if seen != id {
		t.Fatalf("tenant setter: got %v, want %v", seen, id)
	}
}
