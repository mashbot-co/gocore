package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())

	var seen string
	r.GET("/x", func(c *gin.Context) {
		v, _ := c.Get("request_id")
		seen, _ = v.(string)
		c.Status(200)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if seen == "" {
		t.Fatal("expected request_id to be set in context")
	}
	if _, err := uuid.Parse(seen); err != nil {
		t.Errorf("generated request_id is not a UUID: %q (%v)", seen, err)
	}
	if got := w.Header().Get(RequestIDHeader); got != seen {
		t.Errorf("response header %s: got %q, want %q", RequestIDHeader, got, seen)
	}
}

func TestRequestID_PreservesClientSupplied(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())

	const supplied = "client-supplied-id-1234"
	var seen string
	r.GET("/x", func(c *gin.Context) {
		v, _ := c.Get("request_id")
		seen, _ = v.(string)
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, supplied)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if seen != supplied {
		t.Errorf("context request_id: got %q, want %q", seen, supplied)
	}
	if got := w.Header().Get(RequestIDHeader); got != supplied {
		t.Errorf("response header: got %q, want %q", got, supplied)
	}
}
