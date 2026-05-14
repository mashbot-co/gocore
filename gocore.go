// Package gocore exposes consumer-level configuration for the shared
// primitives in this module. Sub-packages (middleware, auth, etc.) read
// from this config at request time so that things like stub-header names,
// JWT issuers, and per-product scope vocabulary can vary per consumer
// without forking gocore.
//
// Consumers call Init exactly once at boot. All fields are optional —
// reasonable defaults apply for anything left empty.
//
// Example (from iro/studio, which uses project-scoping):
//
//	gocore.Init(gocore.Config{
//	    Name:             "iro-studio",
//	    StubHeaderPrefix: "X-Iro-",
//	    JWTIssuer:        "iro-studio",
//	    ScopeName:        "project",   // derives ScopeIDClaim=project_id, ScopeIDArg=projectId
//	})
//
// Example (mashbot, tenant-scoping):
//
//	gocore.Init(gocore.Config{
//	    Name:      "mashbot",
//	    ScopeName: "tenant",
//	})
//
// The defaults are intentionally generic so a consumer that never calls
// Init still gets a working stack — useful in tests and one-off scripts.
package gocore

import "sync"

// Config carries the consumer-level metadata gocore needs to brand its
// behavior. Add fields here sparingly — the goal is to keep this small.
type Config struct {
	// Name is the consumer's short identifier (e.g. "iro-studio"). Used
	// as a default for JWTIssuer when that field is empty.
	Name string

	// StubHeaderPrefix is the prefix for dev-mode stub headers, e.g.
	// "X-Iro-" yields "X-Iro-User-Id". Defaults to "X-Stub-".
	StubHeaderPrefix string

	// JWTIssuer is the value stamped into the `iss` claim of consumer-
	// signed tokens. Defaults to Name (or "gocore" if Name is also empty).
	JWTIssuer string

	// ScopeName names the per-request authorization scope concept this
	// consumer uses (e.g. "project" for iro, "tenant" for mashbot). When
	// set, drives reasonable defaults for ScopeIDClaim and ScopeIDArg.
	// Defaults to empty (no scope concept — auth is user-only).
	ScopeName string

	// ScopeIDClaim is the JSON key for the scope ID in our minted JWT
	// claims (e.g. "project_id"). Defaults to ScopeName + "_id".
	ScopeIDClaim string

	// ScopeIDArg is the GraphQL field argument name that identifies the
	// target scope on project-/tenant-/workspace-scoped operations (e.g.
	// "projectId"). Defaults to ScopeName + "Id".
	ScopeIDArg string
}

var (
	mu      sync.RWMutex
	current = defaults()
)

func defaults() Config {
	return Config{
		Name:             "gocore",
		StubHeaderPrefix: "X-Stub-",
		JWTIssuer:        "gocore",
		ScopeName:        "",
		ScopeIDClaim:     "",
		ScopeIDArg:       "",
	}
}

// Init stores the supplied configuration. Subsequent calls to the getters
// return the new values. Missing fields are filled with sensible defaults.
// Idempotent — callers can re-Init in tests without leaking state via
// Reset (below).
func Init(cfg Config) {
	if cfg.StubHeaderPrefix == "" {
		cfg.StubHeaderPrefix = "X-Stub-"
	}
	if cfg.Name == "" {
		cfg.Name = "gocore"
	}
	if cfg.JWTIssuer == "" {
		cfg.JWTIssuer = cfg.Name
	}
	if cfg.ScopeName != "" {
		if cfg.ScopeIDClaim == "" {
			cfg.ScopeIDClaim = cfg.ScopeName + "_id"
		}
		if cfg.ScopeIDArg == "" {
			cfg.ScopeIDArg = cfg.ScopeName + "Id"
		}
	}
	mu.Lock()
	current = cfg
	mu.Unlock()
}

// Reset restores all defaults. Test helper.
func Reset() {
	mu.Lock()
	current = defaults()
	mu.Unlock()
}

// Name returns the configured consumer name.
func Name() string {
	mu.RLock()
	defer mu.RUnlock()
	return current.Name
}

// StubHeaderPrefix returns the configured dev-mode stub header prefix.
func StubHeaderPrefix() string {
	mu.RLock()
	defer mu.RUnlock()
	return current.StubHeaderPrefix
}

// JWTIssuer returns the configured JWT issuer string.
func JWTIssuer() string {
	mu.RLock()
	defer mu.RUnlock()
	return current.JWTIssuer
}

// ScopeName returns the configured per-request scope concept name
// (empty when the consumer has no scope concept).
func ScopeName() string {
	mu.RLock()
	defer mu.RUnlock()
	return current.ScopeName
}

// ScopeIDClaim returns the JWT-claim key that identifies the request's
// target scope (e.g. "project_id"). Empty if ScopeName is empty.
func ScopeIDClaim() string {
	mu.RLock()
	defer mu.RUnlock()
	return current.ScopeIDClaim
}

// ScopeIDArg returns the GraphQL field-argument name carrying the scope ID
// on scoped operations (e.g. "projectId"). Empty if ScopeName is empty.
func ScopeIDArg() string {
	mu.RLock()
	defer mu.RUnlock()
	return current.ScopeIDArg
}
