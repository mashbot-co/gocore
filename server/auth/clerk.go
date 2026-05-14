package auth

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// VerifyClerkToken validates an inbound Clerk session JWT against Clerk's
// published JWKS and extracts the identity bits we need. Returns an error
// if CLERK_JWKS_URL is unset, the JWKS fetch fails, the signature is bad,
// or the standard claims (exp etc.) are invalid.
//
// Optional env-driven assertions:
//
//   - CLERK_ISSUER:   if set, the token's `iss` claim must match exactly.
//   - CLERK_AUDIENCE: if set, the token's `aud` claim must include this value.
//
// Test/integration callers can override the JWKS source by replacing
// clerkKeyfuncOverride before calling.
func VerifyClerkToken(ctx context.Context, tokenString string) (*IDPIdentity, error) {
	kf, err := clerkKeyfunc(ctx)
	if err != nil {
		return nil, err
	}

	tok, err := jwt.Parse(tokenString, kf)
	if err != nil {
		return nil, fmt.Errorf("clerk: parse token: %w", err)
	}
	if !tok.Valid {
		return nil, fmt.Errorf("clerk: invalid token")
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("clerk: unexpected claims type")
	}

	if expected := os.Getenv("CLERK_ISSUER"); expected != "" {
		iss, _ := claims["iss"].(string)
		if iss != expected {
			return nil, fmt.Errorf("clerk: issuer mismatch: got %q, want %q", iss, expected)
		}
	}
	if expected := os.Getenv("CLERK_AUDIENCE"); expected != "" {
		if !audienceMatches(claims["aud"], expected) {
			return nil, fmt.Errorf("clerk: audience mismatch: got %v, want %q", claims["aud"], expected)
		}
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("clerk: token missing `sub` claim")
	}

	id := &IDPIdentity{Subject: sub}
	id.Email, _ = claims["email"].(string)
	id.OrgID, _ = claims["org_id"].(string)
	id.Name, _ = claims["name"].(string)
	id.FirstName, _ = claims["first_name"].(string)
	id.LastName, _ = claims["last_name"].(string)

	// image_url is the typical Clerk template claim; `picture` is the
	// OIDC-standard alternative. Prefer image_url, fall back to picture.
	if v, _ := claims["image_url"].(string); v != "" {
		id.ImageURL = v
	} else {
		id.ImageURL, _ = claims["picture"].(string)
	}

	return id, nil
}

// audienceMatches handles both single-string and []string `aud` shapes —
// the JWT spec allows either, and Clerk emits the string form, but defensive.
func audienceMatches(aud any, expected string) bool {
	switch v := aud.(type) {
	case string:
		return v == expected
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == expected {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s == expected {
				return true
			}
		}
	}
	return false
}

// clerkKeyfunc returns a jwt.Keyfunc backed by Clerk's JWKS endpoint. The
// underlying keyfunc.Keyfunc handles caching + background refresh per its
// own policies; we just lazy-init it once per process and reuse.
var (
	clerkKfMu       sync.Mutex
	clerkKfInstance jwt.Keyfunc
)

// clerkKeyfuncOverride is the test hook. When non-nil, clerkKeyfunc returns
// it directly without consulting the env or hitting the network.
var clerkKeyfuncOverride jwt.Keyfunc

func clerkKeyfunc(ctx context.Context) (jwt.Keyfunc, error) {
	if clerkKeyfuncOverride != nil {
		return clerkKeyfuncOverride, nil
	}

	clerkKfMu.Lock()
	defer clerkKfMu.Unlock()

	if clerkKfInstance != nil {
		return clerkKfInstance, nil
	}

	url := os.Getenv("CLERK_JWKS_URL")
	if url == "" {
		return nil, fmt.Errorf("clerk: CLERK_JWKS_URL not set")
	}

	kf, err := keyfunc.NewDefaultCtx(ctx, []string{url})
	if err != nil {
		return nil, fmt.Errorf("clerk: build JWKS keyfunc: %w", err)
	}
	clerkKfInstance = kf.Keyfunc
	return clerkKfInstance, nil
}

// SetClerkKeyfuncForTest installs a deterministic jwt.Keyfunc, bypassing
// the JWKS network fetch. Returns a cleanup func that restores the previous
// override (or nil) so tests can nest cleanly.
func SetClerkKeyfuncForTest(kf jwt.Keyfunc) func() {
	prev := clerkKeyfuncOverride
	clerkKeyfuncOverride = kf
	return func() { clerkKeyfuncOverride = prev }
}
