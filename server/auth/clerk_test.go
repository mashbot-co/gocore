package auth

import (
	"context"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// fakeClerkToken signs a Clerk-shaped token using our test keypair and a
// custom kid header, so VerifyClerkToken can validate it via the matching
// jwt.Keyfunc override.
func fakeClerkToken(t *testing.T, priv *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign clerk-shaped token: %v", err)
	}
	return s
}

// keyfuncReturning is a jwt.Keyfunc that always returns the given public
// key — sufficient to test signature validation since we control kid too.
func keyfuncReturning(pub *rsa.PublicKey) jwt.Keyfunc {
	return func(t *jwt.Token) (any, error) { return pub, nil }
}

func TestVerifyClerkToken_ExtractsSubjectEmailOrg(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub":    "user_2abc",
		"email":  "jason@mashbot.com",
		"org_id": "org_xyz",
		"iss":    "https://clerk.example.test",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})

	id, err := VerifyClerkToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("VerifyClerkToken: %v", err)
	}
	if id.Subject != "user_2abc" {
		t.Errorf("Subject: got %q, want %q", id.Subject, "user_2abc")
	}
	if id.Email != "jason@mashbot.com" {
		t.Errorf("Email: got %q", id.Email)
	}
	if id.OrgID != "org_xyz" {
		t.Errorf("OrgID: got %q", id.OrgID)
	}
}

func TestVerifyClerkToken_ExtractsProfileFields(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub":        "user_2abc",
		"email":      "jason@mashbot.com",
		"name":       "Jason Ihaia",
		"first_name": "Jason",
		"last_name":  "Ihaia",
		"image_url":  "https://img.clerk.com/abc",
		"exp":        time.Now().Add(time.Hour).Unix(),
	})

	id, err := VerifyClerkToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("VerifyClerkToken: %v", err)
	}
	if id.Name != "Jason Ihaia" {
		t.Errorf("Name: got %q", id.Name)
	}
	if id.FirstName != "Jason" || id.LastName != "Ihaia" {
		t.Errorf("FirstName/LastName: got %q / %q", id.FirstName, id.LastName)
	}
	if id.ImageURL != "https://img.clerk.com/abc" {
		t.Errorf("ImageURL: got %q", id.ImageURL)
	}
}

func TestVerifyClerkToken_PrefersImageURLOverPicture(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub":       "user_2abc",
		"image_url": "https://img.clerk.com/clerk",
		"picture":   "https://oidc.example/picture",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	id, err := VerifyClerkToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("VerifyClerkToken: %v", err)
	}
	if id.ImageURL != "https://img.clerk.com/clerk" {
		t.Errorf("expected image_url to win over picture, got %q", id.ImageURL)
	}
}

func TestVerifyClerkToken_FallsBackToPicture(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub":     "user_2abc",
		"picture": "https://oidc.example/picture",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	id, err := VerifyClerkToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("VerifyClerkToken: %v", err)
	}
	if id.ImageURL != "https://oidc.example/picture" {
		t.Errorf("expected fallback to picture, got %q", id.ImageURL)
	}
}

func TestVerifyClerkToken_RejectsBadSignature(t *testing.T) {
	priv, _ := freshKeys(t)
	_, otherPub := freshKeys(t)

	// Validator runs against `otherPub` but token was signed with `priv`.
	restore := SetClerkKeyfuncForTest(keyfuncReturning(otherPub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub": "user_2abc",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := VerifyClerkToken(context.Background(), tok); err == nil {
		t.Fatal("expected signature mismatch to fail verification")
	}
}

func TestVerifyClerkToken_RejectsExpired(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub": "user_2abc",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})

	if _, err := VerifyClerkToken(context.Background(), tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestVerifyClerkToken_RejectsMissingSubject(t *testing.T) {
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"email": "x@y.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	if _, err := VerifyClerkToken(context.Background(), tok); err == nil {
		t.Fatal("expected missing sub to fail")
	}
}

func TestVerifyClerkToken_IssuerMismatch(t *testing.T) {
	t.Setenv("CLERK_ISSUER", "https://expected.clerk.test")
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub": "user_2abc",
		"iss": "https://wrong.clerk.test",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := VerifyClerkToken(context.Background(), tok); err == nil {
		t.Fatal("expected issuer mismatch to fail")
	}
}

func TestVerifyClerkToken_AudienceMatchesString(t *testing.T) {
	t.Setenv("CLERK_AUDIENCE", "iro-studio")
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub": "user_2abc",
		"aud": "iro-studio",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := VerifyClerkToken(context.Background(), tok); err != nil {
		t.Fatalf("VerifyClerkToken: %v", err)
	}
}

func TestVerifyClerkToken_AudienceMismatch(t *testing.T) {
	t.Setenv("CLERK_AUDIENCE", "iro-studio")
	priv, pub := freshKeys(t)
	restore := SetClerkKeyfuncForTest(keyfuncReturning(pub))
	defer restore()

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub": "user_2abc",
		"aud": "some-other-app",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := VerifyClerkToken(context.Background(), tok); err == nil {
		t.Fatal("expected audience mismatch to fail")
	}
}

func TestAudienceMatches_AllShapes(t *testing.T) {
	if !audienceMatches("foo", "foo") {
		t.Error("string match")
	}
	if !audienceMatches([]string{"a", "foo", "b"}, "foo") {
		t.Error("[]string match")
	}
	if !audienceMatches([]any{"a", "foo"}, "foo") {
		t.Error("[]any match")
	}
	if audienceMatches([]any{"a", "b"}, "foo") {
		t.Error("[]any no-match should be false")
	}
	if audienceMatches(nil, "foo") {
		t.Error("nil should be false")
	}
}
