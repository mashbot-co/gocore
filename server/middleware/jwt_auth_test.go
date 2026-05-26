package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	gocoreauth "github.com/mashbot-co/gocore/server/auth"
)

func TestJWTAuth(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	restore := gocoreauth.SetKeysForTest(priv, &priv.PublicKey)
	defer restore()

	factory := func() jwt.Claims { return &jwt.RegisteredClaims{} }
	build := func(handler func(*gin.Context, jwt.Claims)) *gin.Engine {
		r := gin.New()
		r.Use(JWTAuth(factory, handler))
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
		return r
	}

	t.Run("missing header rejects", func(t *testing.T) {
		w := httptest.NewRecorder()
		build(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", w.Code)
		}
	})

	t.Run("invalid token rejects", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", "Bearer not.a.real.token")
		w := httptest.NewRecorder()
		build(nil).ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", w.Code)
		}
	})

	t.Run("valid token passes and runs claims handler", func(t *testing.T) {
		tok, err := gocoreauth.SignToken(&jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		})
		if err != nil {
			t.Fatal(err)
		}
		called := false
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		build(func(*gin.Context, jwt.Claims) { called = true }).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", w.Code)
		}
		if !called {
			t.Fatal("claims handler was not invoked")
		}
	})
}
