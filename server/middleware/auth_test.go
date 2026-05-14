package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAuth_RejectsInReleaseMode(t *testing.T) {
	t.Setenv("GIN_MODE", "release")

	r := gin.New()
	r.Use(Auth())
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_AcceptsStubUserInDevMode(t *testing.T) {
	t.Setenv("GIN_MODE", "debug")

	wantUser := uuid.New()

	r := gin.New()
	r.Use(Auth())
	var seenUser uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		if v, ok := c.Get("user_id"); ok {
			seenUser, _ = v.(uuid.UUID)
		}
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Stub-User-Id", wantUser.String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 in dev, got %d", w.Code)
	}
	if seenUser != wantUser {
		t.Errorf("user: got %v, want %v", seenUser, wantUser)
	}
}

func TestAuth_IgnoresInvalidStubHeader(t *testing.T) {
	t.Setenv("GIN_MODE", "debug")

	r := gin.New()
	r.Use(Auth())
	var hasUser bool
	r.GET("/x", func(c *gin.Context) {
		_, hasUser = c.Get("user_id")
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Stub-User-Id", "not-a-uuid")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if hasUser {
		t.Error("user_id should not be set when header is malformed")
	}
}

func TestOptionalAuth_NoHeadersStillCallsNext(t *testing.T) {
	r := gin.New()
	r.Use(OptionalAuth())
	called := false
	r.GET("/x", func(c *gin.Context) {
		called = true
		c.Status(200)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if !called {
		t.Fatal("handler was not invoked")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestOptionalAuth_PicksUpStubUser(t *testing.T) {
	wantUser := uuid.New()

	r := gin.New()
	r.Use(OptionalAuth())
	var seenUser uuid.UUID
	r.GET("/x", func(c *gin.Context) {
		if v, ok := c.Get("user_id"); ok {
			seenUser, _ = v.(uuid.UUID)
		}
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Stub-User-Id", wantUser.String())
	r.ServeHTTP(httptest.NewRecorder(), req)

	if seenUser != wantUser {
		t.Errorf("user: got %v, want %v", seenUser, wantUser)
	}
}

func TestOptionalAuth_BearerHeaderIsTolerated(t *testing.T) {
	// Real verification isn't wired yet — middleware should ignore the bearer
	// rather than panicking or rejecting.
	r := gin.New()
	r.Use(OptionalAuth())
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.value")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
