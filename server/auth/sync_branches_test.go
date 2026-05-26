package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// setupSyncAuth installs deterministic signing keys + a Clerk keyfunc override
// for the duration of the test and returns the private key for minting tokens.
func setupSyncAuth(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	priv, pub := freshKeys(t)
	t.Cleanup(SetClerkKeyfuncForTest(keyfuncReturning(pub)))
	t.Cleanup(SetKeysForTest(priv, pub))
	return priv
}

func doSync(r *gin.Engine, bearer, body string) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/sync", rdr)
	req.Header.Set("Authorization", "Bearer "+bearer)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func mintClerk(t *testing.T, priv *rsa.PrivateKey, claims jwt.MapClaims) string {
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	return fakeClerkToken(t, priv, "test-kid", claims)
}

func TestSyncHandler_EmailMissing(t *testing.T) {
	priv := setupSyncAuth(t)
	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, validHooks()))
	w := doSync(r, mintClerk(t, priv, jwt.MapClaims{"sub": "u"}), "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 for missing email", w.Code)
	}
}

func TestSyncHandler_FindOrCreateUserError(t *testing.T) {
	priv := setupSyncAuth(t)
	h := validHooks()
	h.FindOrCreateUser = func(*gorm.DB, *IDPIdentity) (SyncUser, error) { return nil, errors.New("boom") }
	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, h))
	w := doSync(r, mintClerk(t, priv, jwt.MapClaims{"sub": "u", "email": "a@b.c"}), "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", w.Code)
	}
}

func TestSyncHandler_ListMembershipsError(t *testing.T) {
	priv := setupSyncAuth(t)
	h := validHooks()
	h.ListMemberships = func(*gorm.DB, uuid.UUID) ([]SyncMembership, error) { return nil, errors.New("boom") }
	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, h))
	w := doSync(r, mintClerk(t, priv, jwt.MapClaims{"sub": "u", "email": "a@b.c"}), "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", w.Code)
	}
}

func TestSyncHandler_ScopeOverrideMiss403(t *testing.T) {
	priv := setupSyncAuth(t)
	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, validHooks()))
	body := `{"scopeId":"` + uuid.New().String() + `"}` // not in the user's memberships
	w := doSync(r, mintClerk(t, priv, jwt.MapClaims{"sub": "u", "email": "a@b.c"}), body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403 for non-member scope override", w.Code)
	}
}

func TestSyncHandler_NoMembershipsStillMintsToken(t *testing.T) {
	priv := setupSyncAuth(t)
	h := validHooks()
	h.ListMemberships = func(*gorm.DB, uuid.UUID) ([]SyncMembership, error) { return nil, nil }
	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, h))
	w := doSync(r, mintClerk(t, priv, jwt.MapClaims{"sub": "u", "email": "a@b.c"}), "")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (token minted with no current scope)", w.Code)
	}
}

func TestVerifyClerkToken_IssuerMismatchAndMissingSub(t *testing.T) {
	priv, pub := freshKeys(t)
	defer SetClerkKeyfuncForTest(keyfuncReturning(pub))()

	t.Setenv("CLERK_ISSUER", "https://expected.test")
	bad := fakeClerkToken(t, priv, "k", jwt.MapClaims{"sub": "u", "iss": "https://wrong.test", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := VerifyClerkToken(context.Background(), bad); err == nil {
		t.Error("expected issuer mismatch error")
	}

	t.Setenv("CLERK_ISSUER", "")
	noSub := fakeClerkToken(t, priv, "k", jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})
	if _, err := VerifyClerkToken(context.Background(), noSub); err == nil {
		t.Error("expected missing-sub error")
	}
}

func TestParsePrivateKey_PKCS1(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PrivateKey(priv)
	p := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	got, err := parsePrivateKey(p)
	if err != nil || got == nil {
		t.Fatalf("PKCS1 parse: got %v, err %v", got, err)
	}
}
