package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore/connection"
)

func TestTenantIDFromContext_ReturnsNilWhenAbsent(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	if got := TenantIDFromContext(c); got != uuid.Nil {
		t.Errorf("expected uuid.Nil, got %v", got)
	}
}

func TestTenantIDFromContext_ReturnsNilOnWrongType(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("tenant_id", "not-a-uuid")

	if got := TenantIDFromContext(c); got != uuid.Nil {
		t.Errorf("expected uuid.Nil on type mismatch, got %v", got)
	}
}

func TestTenantIDFromContext_ReturnsValue(t *testing.T) {
	want := uuid.New()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("tenant_id", want)

	if got := TenantIDFromContext(c); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUserIDFromContext_ReturnsNilWhenAbsent(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	if got := UserIDFromContext(c); got != uuid.Nil {
		t.Errorf("expected uuid.Nil, got %v", got)
	}
}

func TestUserIDFromContext_ReturnsNilOnWrongType(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("user_id", 42)

	if got := UserIDFromContext(c); got != uuid.Nil {
		t.Errorf("expected uuid.Nil on type mismatch, got %v", got)
	}
}

func TestUserIDFromContext_ReturnsValue(t *testing.T) {
	want := uuid.New()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("user_id", want)

	if got := UserIDFromContext(c); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGormContext_ThreadsBothIDsIntoRequestContext(t *testing.T) {
	wantTenant := uuid.New()
	wantUser := uuid.New()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("tenant_id", wantTenant)
	c.Set("user_id", wantUser)

	GormContext(c)

	ctx := c.Request.Context()
	if got := connection.CurrentTenant(ctx); got != wantTenant {
		t.Errorf("tenant in context: got %v, want %v", got, wantTenant)
	}
	if got := connection.CurrentUser(ctx); got != wantUser {
		t.Errorf("user in context: got %v, want %v", got, wantUser)
	}
}

func TestGormContext_OmitsZeroValues(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	// No tenant_id or user_id set.

	GormContext(c)

	ctx := c.Request.Context()
	if got := connection.CurrentTenant(ctx); got != uuid.Nil {
		t.Errorf("expected nil tenant, got %v", got)
	}
	if got := connection.CurrentUser(ctx); got != uuid.Nil {
		t.Errorf("expected nil user, got %v", got)
	}
}

func TestInjectDBContext_ThreadsIDsAndCallsNext(t *testing.T) {
	wantTenant := uuid.New()
	wantUser := uuid.New()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", wantTenant)
		c.Set("user_id", wantUser)
		c.Next()
	})
	r.Use(InjectDBContext())

	var sawTenant, sawUser uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		ctx := c.Request.Context()
		sawTenant = connection.CurrentTenant(ctx)
		sawUser = connection.CurrentUser(ctx)
		c.Status(200)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if sawTenant != wantTenant {
		t.Errorf("tenant in handler ctx: got %v, want %v", sawTenant, wantTenant)
	}
	if sawUser != wantUser {
		t.Errorf("user in handler ctx: got %v, want %v", sawUser, wantUser)
	}
}

func TestInjectDBContext_NoIDsLeavesContextUntouched(t *testing.T) {
	r := gin.New()
	r.Use(InjectDBContext())

	var sawTenant, sawUser uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		ctx := c.Request.Context()
		sawTenant = connection.CurrentTenant(ctx)
		sawUser = connection.CurrentUser(ctx)
		c.Status(200)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if sawTenant != uuid.Nil {
		t.Errorf("expected nil tenant, got %v", sawTenant)
	}
	if sawUser != uuid.Nil {
		t.Errorf("expected nil user, got %v", sawUser)
	}
}
