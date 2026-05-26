package auth

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SyncUser is the minimum surface gocore needs from a consumer's user
// type to drive /auth/sync. iro implements this on models.User (project
// is the scope); mashbot would implement it on its User with tenant as
// the scope. Methods are prefixed `Get` to avoid collisions with the
// matching struct fields.
type SyncUser interface {
	GetUserID() uuid.UUID
	GetLastScopeID() *uuid.UUID
}

// SyncMembership is the minimum surface a scope-membership row exposes
// to gocore — enough to pick a current membership, build the response
// catalog, and resolve the role for our minted JWT.
type SyncMembership interface {
	GetScopeID() uuid.UUID
	GetScopeName() string
	GetRole() string
}

// SyncRequest is the optional body — present only when switching scopes
// (e.g. picking a different project). On initial sign-in the body is
// empty and the handler picks via user.LastScopeID → first membership.
type SyncRequest struct {
	ScopeID *uuid.UUID `json:"scopeId"`
}

// SyncResponse is what /auth/sync hands back to the client. User is
// `any` so the consumer's concrete User type serialises naturally —
// gocore doesn't impose a User wire shape, only its read methods.
type SyncResponse struct {
	User           any               `json:"user"`
	Token          string            `json:"token"`
	CurrentScopeID *uuid.UUID        `json:"currentScopeId"`
	CurrentRole    *string           `json:"currentRole"`
	Memberships    []MembershipEntry `json:"memberships"`
}

// MembershipEntry is one row of the scope-switcher catalog returned to
// the client. Scope-named so it's product-agnostic (project for iro,
// tenant for mashbot, etc.).
type MembershipEntry struct {
	ScopeID   uuid.UUID `json:"scopeId"`
	ScopeName string    `json:"scopeName"`
	Role      string    `json:"role"`
}

// SyncOption configures optional behaviour on SyncHandler. Use the
// `With*` constructors below — direct field manipulation isn't part of
// the API.
type SyncOption func(*syncConfig)

// SessionCookie describes the cross-subdomain session cookie set on
// successful sync. Passed via WithSessionCookie.
type SessionCookie struct {
	// Name is the cookie name (e.g. "iro_session"). Required.
	Name string
	// Domain sets Domain= so the cookie is visible to every subdomain
	// of the parent (e.g. ".iro.local" → www.iro.local + app.iro.local).
	// Required for cross-subdomain use; omit for a host-only cookie.
	Domain string
	// Path defaults to "/" when empty.
	Path string
	// MaxAge in seconds. 0 falls back to a session cookie (cleared on
	// browser close); use a value that matches the JWT's exp.
	MaxAge int
	// Secure must be true on HTTPS; browsers reject Secure cookies on
	// plain HTTP, so dev with http needs false.
	Secure bool
	// HttpOnly hides the cookie from document.cookie. Strongly
	// recommended true so XSS can't lift the session.
	HttpOnly bool
	// SameSite controls cross-site send behaviour. Lax is the right
	// default for a typical sign-in → dashboard flow on a sibling
	// subdomain (top-level navigation is allowed).
	SameSite http.SameSite
}

type syncConfig struct {
	cookie *SessionCookie
}

// WithSessionCookie tells SyncHandler to set a Set-Cookie header on
// successful sync, alongside the JSON response. The cookie value is the
// same token returned in the body.
func WithSessionCookie(c SessionCookie) SyncOption {
	return func(cfg *syncConfig) { cfg.cookie = &c }
}

// ClearSessionCookieHandler returns a gin handler that emits a Set-Cookie
// for the configured session cookie with MaxAge=-1, telling the browser
// to delete it immediately. Mount on a sign-out endpoint; pair with the
// same SessionCookie value used in WithSessionCookie so Name/Domain/Path
// match (browsers won't delete a cookie unless the clearing Set-Cookie's
// scope matches the original). No auth required — anyone who has the
// cookie already has the session, so clearing their own is a no-op risk.
func ClearSessionCookieHandler(ck SessionCookie) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := ck.Path
		if path == "" {
			path = "/"
		}
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     ck.Name,
			Value:    "",
			Path:     path,
			Domain:   ck.Domain,
			MaxAge:   -1,
			Secure:   ck.Secure,
			HttpOnly: ck.HttpOnly,
			SameSite: ck.SameSite,
		})
		c.Status(http.StatusNoContent)
	}
}

// SyncHooks is the consumer-supplied glue for SyncHandler. The handler
// owns the flow (verify Clerk, parse body, mint JWT, format response);
// these hooks own the model-touching parts (queries, updates, claims
// construction).
type SyncHooks struct {
	// FindOrCreateUser materializes a SyncUser from the verified IDP
	// identity. Typical implementation: lookup by sso_id, fall back to
	// email (back-link), create on miss. Email/name/avatar refresh
	// policy is the consumer's choice.
	FindOrCreateUser func(db *gorm.DB, identity *IDPIdentity) (SyncUser, error)

	// ListMemberships returns the user's active memberships across
	// every scope they belong to. The handler iterates this list to
	// build the response catalog and to find the membership for the
	// requested current scope.
	ListMemberships func(db *gorm.DB, userID uuid.UUID) ([]SyncMembership, error)

	// UpdateLastScope persists "current scope" on the user row so a
	// follow-up sync with no body lands on the same scope. Called only
	// when the resolved current scope differs from the user's previous.
	UpdateLastScope func(db *gorm.DB, userID, scopeID uuid.UUID) error

	// BuildClaims constructs the consumer's Claims type. gocore signs
	// whatever this returns. The membership pointer is nil when the
	// user has no scopes (first sign-in before any project exists).
	BuildClaims func(user SyncUser, membership SyncMembership) jwt.Claims
}

// SyncHandler wires the hooks into a gin.HandlerFunc. Mount it via
// `r.POST("/auth/sync", auth.SyncHandler(db, hooks))`. Pass options
// (e.g. WithSessionCookie) to layer in optional behaviour.
func SyncHandler(db *gorm.DB, hooks SyncHooks, opts ...SyncOption) gin.HandlerFunc {
	if hooks.FindOrCreateUser == nil || hooks.ListMemberships == nil ||
		hooks.UpdateLastScope == nil || hooks.BuildClaims == nil {
		log.Fatal("auth.SyncHandler: all SyncHooks fields must be set")
	}
	cfg := syncConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(c *gin.Context) {
		clerkToken, err := BearerFrom(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		identity, err := VerifyClerkToken(c.Request.Context(), clerkToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "clerk verification failed: " + err.Error()})
			return
		}
		if identity.Email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "clerk token missing email claim — configure the Clerk session to include email"})
			return
		}

		var req SyncRequest
		_ = c.ShouldBindJSON(&req)

		user, err := hooks.FindOrCreateUser(db, identity)
		if err != nil {
			log.Printf("[auth/sync] materialize user: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to materialize user"})
			return
		}

		memberships, err := hooks.ListMemberships(db, user.GetUserID())
		if err != nil {
			log.Printf("[auth/sync] list memberships: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list memberships"})
			return
		}

		current, err := PickCurrent(memberships, req.ScopeID, user.GetLastScopeID())
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		if current != nil {
			cur := *current
			scope := cur.GetScopeID()
			last := user.GetLastScopeID()
			if last == nil || *last != scope {
				if err := hooks.UpdateLastScope(db, user.GetUserID(), scope); err != nil {
					log.Printf("[auth/sync] update last scope: %v", err)
				}
			}
		}

		var claims jwt.Claims
		if current != nil {
			claims = hooks.BuildClaims(user, *current)
		} else {
			claims = hooks.BuildClaims(user, nil)
		}
		token, err := SignToken(claims)
		if err != nil {
			log.Printf("[auth/sync] sign token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mint token"})
			return
		}

		resp := SyncResponse{
			User:        user,
			Token:       token,
			Memberships: toEntries(memberships),
		}
		if current != nil {
			cur := *current
			scope := cur.GetScopeID()
			role := cur.GetRole()
			resp.CurrentScopeID = &scope
			resp.CurrentRole = &role
		}

		if cfg.cookie != nil {
			ck := cfg.cookie
			path := ck.Path
			if path == "" {
				path = "/"
			}
			// gin's SetCookie doesn't expose SameSite, so set it on the
			// underlying ResponseWriter via http.SetCookie directly.
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     ck.Name,
				Value:    token,
				Path:     path,
				Domain:   ck.Domain,
				MaxAge:   ck.MaxAge,
				Secure:   ck.Secure,
				HttpOnly: ck.HttpOnly,
				SameSite: ck.SameSite,
			})
		}

		c.JSON(http.StatusOK, resp)
	}
}

// BearerFrom extracts the bearer token from the request's Authorization
// header. Returns an error suitable for surfacing as 401 if the header
// is missing or malformed.
func BearerFrom(c *gin.Context) (string, error) {
	h := c.GetHeader("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", errors.New("missing or malformed Authorization header")
	}
	return strings.TrimPrefix(h, "Bearer "), nil
}

// ComposeName builds a display-name string from whatever name claims the
// IDP gave us. Prefers an explicit `name` claim; falls back to
// "FirstName LastName"; returns "" when neither is available. Useful
// for the consumer's findOrCreateUser hook when seeding the local
// User row's display name.
func ComposeName(identity *IDPIdentity) string {
	if identity.Name != "" {
		return identity.Name
	}
	first := strings.TrimSpace(identity.FirstName)
	last := strings.TrimSpace(identity.LastName)
	switch {
	case first != "" && last != "":
		return first + " " + last
	case first != "":
		return first
	default:
		return last
	}
}

// PtrIfNotEmpty returns &s when s is non-empty, nil otherwise. Sugar
// for populating *string fields on User-like structs without nil-check
// boilerplate at every call site.
func PtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// PickCurrent picks the current-scope membership using the standard
// priority: explicit body override → user's persisted last-scope →
// first available membership → nil (no scopes yet). Override referring
// to a scope the user isn't a member of is rejected with an error.
func PickCurrent(memberships []SyncMembership, override, lastScope *uuid.UUID) (*SyncMembership, error) {
	if override != nil {
		for i := range memberships {
			if memberships[i].GetScopeID() == *override {
				return &memberships[i], nil
			}
		}
		return nil, errors.New("no active membership for the requested scope")
	}
	if lastScope != nil {
		for i := range memberships {
			if memberships[i].GetScopeID() == *lastScope {
				return &memberships[i], nil
			}
		}
	}
	if len(memberships) > 0 {
		return &memberships[0], nil
	}
	return nil, nil
}

// toEntries flattens []SyncMembership into the JSON-friendly response
// shape. Pure transformation — no DB access, no consumer code.
func toEntries(ms []SyncMembership) []MembershipEntry {
	out := make([]MembershipEntry, 0, len(ms))
	for _, m := range ms {
		out = append(out, MembershipEntry{
			ScopeID:   m.GetScopeID(),
			ScopeName: m.GetScopeName(),
			Role:      m.GetRole(),
		})
	}
	return out
}
