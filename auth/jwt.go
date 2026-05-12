// Package auth handles identity verification against an OIDC-style provider
// (Clerk by default — but the surface is generic enough that the same shape
// works for WorkOS, Auth0, etc).
//
// The current state is a stub. When Clerk integration lands:
//
//   - InitKeys fetches and caches the provider's JWKS from the configured
//     Frontend API URL.
//   - ValidateToken verifies an incoming JWT against that JWKS, returning the
//     claims (UserID, TenantID derived from the org, plus the raw subject and
//     org identifiers for audit/sync).
//
// Consumers materialize a Claims into their own domain User/Tenant rows in
// their own /auth/sync handler — that handler stays per-project because the
// User model varies. See <consumer>/apis/v1/core/auth/sync.go.
//
// Configuration via env / SSM:
//
//	CLERK_JWKS_URL    — e.g. https://clerk.iro.studio/.well-known/jwks.json
//	CLERK_ISSUER      — e.g. https://clerk.iro.studio
//	CLERK_AUDIENCE    — usually the API URL or a custom audience claim
package auth

import (
	"fmt"

	"github.com/google/uuid"
)

// Claims is what a verified Clerk JWT yields, mapped onto our domain.
type Claims struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
	Email    string
	Subject  string // raw provider user identifier ("user_xxx")
	OrgID    string // raw provider org identifier ("org_xxx"), if present
}

// InitKeys is called from the consumer's main.go at boot. Currently a no-op
// pending Clerk integration. Will fetch and cache the JWKS once wired.
func InitKeys() error {
	return nil
}

// ValidateToken verifies an incoming JWT and returns the resolved claims.
// Currently returns an error pending Clerk integration.
func ValidateToken(token string) (*Claims, error) {
	return nil, fmt.Errorf("auth.ValidateToken: not yet implemented")
}
