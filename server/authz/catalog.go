package authz

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Catalog is the universal scope-catalog primitive — a set of scopes plus
// role bundles that map roles to scope patterns. Consumers supply the data
// (iro/shared/policy embeds iro's catalog, mashbot would embed its own);
// the parsing, wildcard expansion, and grant-check logic live here so every
// consumer benefits from the same algorithm.
//
// Catalog satisfies the Policy interface this package consumes for authz
// decisions — pass a loaded *Catalog directly into AuthorizeField.
type Catalog struct {
	Scopes map[string]ScopeDef `yaml:"scopes"`
	Roles  map[string][]string `yaml:"roles"`
}

// ScopeDef is a single scope's metadata. Only `description` today; room
// for tagging / categorisation without restructuring callers.
type ScopeDef struct {
	Description string `yaml:"description"`
}

// ParseCatalog parses raw scopes.yml bytes. Returns a populated Catalog
// after validating that every concrete (non-wildcard) scope referenced in
// a role bundle exists in the scope set — guards against typos that would
// otherwise silently grant nothing.
func ParseCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("authz: parse scope catalog: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// MustParseCatalog is ParseCatalog with panic-on-error. Call from main.go
// init so configuration mistakes fail loudly at boot.
func MustParseCatalog(data []byte) *Catalog {
	c, err := ParseCatalog(data)
	if err != nil {
		panic(err)
	}
	return c
}

// Grants reports whether the role grants the given scope. Wildcard
// patterns in the role bundle (e.g. `project:*`) match by prefix —
// `project:*` grants `project:read`, `project:write`, and any future
// `project:foo:bar`.
func (c *Catalog) Grants(role, scope string) bool {
	patterns, ok := c.Roles[role]
	if !ok {
		return false
	}
	for _, p := range patterns {
		if matchScope(p, scope) {
			return true
		}
	}
	return false
}

// MatchesAll reports whether the role grants ALL listed scopes. An empty
// required list is satisfied by any role (the "any authenticated user"
// case for fields with no role gate).
func (c *Catalog) MatchesAll(role string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	for _, s := range required {
		if !c.Grants(role, s) {
			return false
		}
	}
	return true
}

// Expand returns the concrete scope set a role grants, with wildcards
// resolved against the catalog. Useful for serialising the caller's
// effective permissions (e.g. sending to a UI to decide which buttons to
// show) or for debugging.
func (c *Catalog) Expand(role string) map[string]struct{} {
	out := map[string]struct{}{}
	patterns, ok := c.Roles[role]
	if !ok {
		return out
	}
	for _, p := range patterns {
		if strings.HasSuffix(p, ":*") {
			prefix := strings.TrimSuffix(p, "*")
			for s := range c.Scopes {
				if strings.HasPrefix(s, prefix) {
					out[s] = struct{}{}
				}
			}
			continue
		}
		out[p] = struct{}{}
	}
	return out
}

// matchScope decides whether a single pattern (from a role bundle) grants
// a single concrete scope. Prefix-glob semantics: `project:*` matches
// `project:read` AND `project:read:detail`.
func matchScope(pattern, scope string) bool {
	if pattern == scope {
		return true
	}
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(scope, prefix)
	}
	return false
}

// validate runs sanity checks on a parsed catalog: every concrete scope
// pattern in a role bundle must point at a real scope. Wildcards are
// tolerated even if they expand to nothing today (future-proofing).
func (c *Catalog) validate() error {
	for role, patterns := range c.Roles {
		for _, p := range patterns {
			if strings.HasSuffix(p, ":*") {
				continue // wildcard, may expand to zero today
			}
			if _, ok := c.Scopes[p]; !ok {
				return fmt.Errorf("authz: role %q references unknown scope %q", role, p)
			}
		}
	}
	return nil
}
