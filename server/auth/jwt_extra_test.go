package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func pemB64Keys(t *testing.T, priv *rsa.PrivateKey) (privB64, pubB64 string) {
	t.Helper()
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return base64.StdEncoding.EncodeToString(privPEM), base64.StdEncoding.EncodeToString(pubPEM)
}

// TestInit_WithEnvKeys covers the env-key path of Init (decodeBase64,
// parsePrivateKey, parsePublicKey, computeKID) plus a sign/verify round-trip.
func TestInit_WithEnvKeys(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privB64, pubB64 := pemB64Keys(t, priv)
	t.Setenv("JWT_PRIVATE_KEY", privB64)
	t.Setenv("JWT_PUBLIC_KEY", pubB64)

	if err := Init(); err != nil {
		t.Fatalf("Init with env keys: %v", err)
	}
	tok, err := SignToken(&jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))})
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if _, err := VerifyToken(tok, &jwt.RegisteredClaims{}); err != nil {
		t.Fatalf("VerifyToken round-trip: %v", err)
	}
}

func TestInit_EphemeralWhenUnset(t *testing.T) {
	t.Setenv("JWT_PRIVATE_KEY", "")
	t.Setenv("JWT_PUBLIC_KEY", "")
	if err := Init(); err != nil {
		t.Fatalf("Init ephemeral: %v", err)
	}
	if PublicKey() == nil {
		t.Fatal("expected an ephemeral public key after Init")
	}
}

func TestInit_BadBase64AndBadPEM(t *testing.T) {
	t.Setenv("JWT_PRIVATE_KEY", "!!!not base64!!!")
	t.Setenv("JWT_PUBLIC_KEY", "!!!not base64!!!")
	if err := Init(); err == nil {
		t.Error("expected decode error for non-base64 keys")
	}

	good := base64.StdEncoding.EncodeToString([]byte("not a pem block"))
	t.Setenv("JWT_PRIVATE_KEY", good)
	t.Setenv("JWT_PUBLIC_KEY", good)
	if err := Init(); err == nil {
		t.Error("expected parse error for non-PEM key material")
	}
}

func TestJWKS_And_Handler(t *testing.T) {
	priv, pub := freshKeys(t)
	defer SetKeysForTest(priv, pub)()

	set := JWKS()
	if len(set.Keys) != 1 {
		t.Fatalf("JWKS keys = %d, want 1", len(set.Keys))
	}
	k := set.Keys[0]
	if k.Kty != "RSA" || k.Alg != "RS256" || k.Use != "sig" || k.N == "" || k.E == "" {
		t.Errorf("unexpected JWK shape: %+v", k)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/jwks", JWKSHandler)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/jwks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("JWKSHandler = %d, want 200", w.Code)
	}
}

func TestJWKS_EmptyWhenUninitialized(t *testing.T) {
	defer SetKeysForTest(nil, nil)()
	if len(JWKS().Keys) != 0 {
		t.Error("expected empty JWKS when uninitialized")
	}
}

func TestVerifyToken_NilKeyAndWrongMethod(t *testing.T) {
	// nil public key → error
	restore := SetKeysForTest(nil, nil)
	if _, err := VerifyToken("whatever", &jwt.RegisteredClaims{}); err == nil {
		t.Error("expected error when public key is uninitialized")
	}
	restore()

	// HS256 token against an RSA verifier → signing-method rejection.
	priv, pub := freshKeys(t)
	defer SetKeysForTest(priv, pub)()
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	s, err := hs.SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyToken(s, &jwt.RegisteredClaims{}); err == nil {
		t.Error("expected rejection of a non-RSA (HS256) token")
	}
}
