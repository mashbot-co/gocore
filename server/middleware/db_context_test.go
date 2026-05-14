package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mashbot-co/gocore/db/connection"
)

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

func TestGormContext_ThreadsUserIntoRequestContext(t *testing.T) {
	wantUser := uuid.New()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("user_id", wantUser)

	GormContext(c)

	if got := connection.CurrentUser(c.Request.Context()); got != wantUser {
		t.Errorf("user in context: got %v, want %v", got, wantUser)
	}
}

func TestGormContext_OmitsZeroValues(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GormContext(c)

	if got := connection.CurrentUser(c.Request.Context()); got != uuid.Nil {
		t.Errorf("expected nil user, got %v", got)
	}
}

func TestInjectDBContext_ThreadsUserAndCallsNext(t *testing.T) {
	wantUser := uuid.New()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", wantUser)
		c.Next()
	})
	r.Use(InjectDBContext())

	var sawUser uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		sawUser = connection.CurrentUser(c.Request.Context())
		c.Status(200)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if sawUser != wantUser {
		t.Errorf("user in handler ctx: got %v, want %v", sawUser, wantUser)
	}
}

func TestInjectDBContext_NoIDsLeavesContextUntouched(t *testing.T) {
	r := gin.New()
	r.Use(InjectDBContext())

	var sawUser uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		sawUser = connection.CurrentUser(c.Request.Context())
		c.Status(200)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if sawUser != uuid.Nil {
		t.Errorf("expected nil user, got %v", sawUser)
	}
}
