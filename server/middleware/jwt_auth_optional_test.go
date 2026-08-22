package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	gocoreauth "github.com/mashbot-co/gocore/server/auth"
)

// signedToken installs a test keypair for the duration of the test and
// returns a token that verifies against it.
func signedToken(t *testing.T, sub string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	t.Cleanup(gocoreauth.SetKeysForTest(key, &key.PublicKey))
	tok, err := gocoreauth.SignToken(&jwt.RegisteredClaims{Subject: sub})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

func optionalRouter(handler func(c *gin.Context, claims jwt.Claims)) *gin.Engine {
	r := gin.New()
	r.Use(JWTAuthOptional(testClaims, handler))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	return r
}

func TestJWTAuthOptional_AnonymousPassesThrough(t *testing.T) {
	w := httptest.NewRecorder()
	optionalRouter(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusOK {
		t.Errorf("anonymous request should pass through, got %d", w.Code)
	}
}

func TestJWTAuthOptional_MalformedHeaderIsRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwdw==")
	w := httptest.NewRecorder()
	optionalRouter(nil).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("non-Bearer header is a broken credential, want 401, got %d", w.Code)
	}
}

func TestJWTAuthOptional_InvalidTokenIsRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not.a.token")
	w := httptest.NewRecorder()
	optionalRouter(nil).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unverifiable token, want 401, got %d", w.Code)
	}
}

func TestJWTAuthOptional_ValidTokenLiftsClaims(t *testing.T) {
	tok := signedToken(t, "user-9")

	var gotSub string
	r := optionalRouter(func(c *gin.Context, claims jwt.Claims) {
		if rc, ok := claims.(*jwt.RegisteredClaims); ok {
			gotSub = rc.Subject
		}
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("valid token, want 200, got %d", w.Code)
	}
	if gotSub != "user-9" {
		t.Errorf("claims handler saw sub %q, want user-9", gotSub)
	}
}

func TestOptionalAuth_ValidBearerInvokesClaimsHandler(t *testing.T) {
	tok := signedToken(t, uuid.NewString())

	handled := false
	r := gin.New()
	r.Use(OptionalAuth(testClaims, func(c *gin.Context, claims jwt.Claims) { handled = true }))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !handled {
		t.Error("claims handler should run for a verified bearer token")
	}
}
