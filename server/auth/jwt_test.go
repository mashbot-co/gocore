package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// testClaims is a stand-in for whatever a consumer would define. It just
// needs to satisfy jwt.Claims, which RegisteredClaims provides via embed.
type testClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role,omitempty"`
	jwt.RegisteredClaims
}

func freshKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k, &k.PublicKey
}

func TestInit_GeneratesEphemeralKeysWhenEnvUnset(t *testing.T) {
	t.Setenv("JWT_PRIVATE_KEY", "")
	t.Setenv("JWT_PUBLIC_KEY", "")
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if PublicKey() == nil {
		t.Fatal("expected ephemeral PublicKey to be set")
	}
}

func TestSignToken_AndVerifyToken_RoundTrip(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetKeysForTest(priv, pub)
	defer restore()

	uid := uuid.New()
	claims := testClaims{
		UserID: uid,
		Role:   "owner",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token, err := SignToken(claims)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	var got testClaims
	if _, err := VerifyToken(token, &got); err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if got.UserID != uid {
		t.Errorf("user_id roundtrip: got %v, want %v", got.UserID, uid)
	}
	if got.Role != "owner" {
		t.Errorf("role roundtrip: got %q, want %q", got.Role, "owner")
	}
}

func TestVerifyToken_RejectsExpired(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetKeysForTest(priv, pub)
	defer restore()

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	token, err := SignToken(claims)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	if _, err := VerifyToken(token, &testClaims{}); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestSignToken_FailsWithoutInit(t *testing.T) {
	restore := SetKeysForTest(nil, nil)
	defer restore()

	if _, err := SignToken(testClaims{}); err == nil {
		t.Fatal("expected SignToken to fail without private key")
	}
}

func TestVerifyToken_RejectsHS256(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetKeysForTest(priv, pub)
	defer restore()

	// Sign with HS256 (wrong family) so the alg check rejects it.
	hsToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("hs256 sign: %v", err)
	}
	if _, err := VerifyToken(hsToken, &testClaims{}); err == nil {
		t.Fatal("expected HS256 token to be rejected")
	}
}

func TestInitKeys_AliasStillWorks(t *testing.T) {
	t.Setenv("JWT_PRIVATE_KEY", "")
	t.Setenv("JWT_PUBLIC_KEY", "")
	if err := InitKeys(); err != nil {
		t.Fatalf("InitKeys: %v", err)
	}
}
