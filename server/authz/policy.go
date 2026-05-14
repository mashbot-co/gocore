// Package authz provides the gocore-side machinery for scope-based
// authorization. The consumer supplies:
//
//   - a Catalog (scopes + role bundles) — typically loaded from a shared
//     policy YAML by iro/shared/policy or its equivalent.
//   - an EndpointPolicy (field → required scopes) — loaded from each API's
//     auth.yml; this package provides the loader.
//   - a RoleResolver that maps "the calling user (+ optional project arg)"
//     to an effective role — typically "owner if is_admin, else look up
//     Membership.role for the projectId arg, else empty role."
//
// gocore plugs the @requireScopes directive into the consumer's gqlgen
// generated.Config; the directive consults Policy + EndpointPolicy +
// RoleResolver at request time to allow or reject each field.
package authz

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Policy is the catalog interface gocore needs to check authz. The
// concrete *Catalog in this package satisfies it; consumers can also
// supply their own implementation (e.g. a remote policy server, an
// in-memory test stub) without depending on the YAML-based loader.
type Policy interface {
	// MatchesAll reports whether the role grants ALL listed scopes. An
	// empty required list is satisfied by any role (including "").
	MatchesAll(role string, scopes []string) bool
}

// EndpointPolicy maps each GraphQL field to its scope requirements, loaded
// from a consumer's apis/v1/<api>/auth.yml. The directive consults it on
// every field resolution to decide what scopes to require.
type EndpointPolicy struct {
	// Fields maps field name (camelCase, matching the GraphQL field name)
	// to its rule. Required scopes are AND'd.
	Fields map[string]FieldRule `yaml:"fields"`
	// Public is the list of field names callable without authentication.
	Public []string `yaml:"public"`

	publicSet map[string]struct{}
}

// FieldRule is a single endpoint's requirements. Today only `scopes` —
// more axes (e.g. rate-limit) can grow here later.
type FieldRule struct {
	Scopes []string `yaml:"scopes"`
}

// LoadEndpointPolicy reads an auth.yml file. Returns a populated
// EndpointPolicy with the public-set pre-indexed for O(1) lookups.
// Convenience wrapper over ParseEndpointPolicy — prefer embedding the YAML
// into the consumer binary and calling ParseEndpointPolicy directly so
// Lambda / container builds don't depend on a runtime file path.
func LoadEndpointPolicy(path string) (*EndpointPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("authz: read %s: %w", path, err)
	}
	return ParseEndpointPolicy(data)
}

// ParseEndpointPolicy parses raw auth.yml bytes — typically supplied by
// the consumer via go:embed so the policy ships with the binary.
func ParseEndpointPolicy(data []byte) (*EndpointPolicy, error) {
	var p EndpointPolicy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("authz: parse endpoint policy: %w", err)
	}
	p.publicSet = make(map[string]struct{}, len(p.Public))
	for _, name := range p.Public {
		p.publicSet[name] = struct{}{}
	}
	return &p, nil
}

// IsPublic reports whether the field is callable without authentication.
func (p *EndpointPolicy) IsPublic(field string) bool {
	_, ok := p.publicSet[field]
	return ok
}

// RequiredScopes returns the scopes the field requires, or (nil, false) if
// the field has no entry in the policy. The directive caller treats "no
// entry, not in public:" as a configuration error at request time — but the
// scanner should already have rejected it at generation time, so this
// branch is belt-and-braces.
func (p *EndpointPolicy) RequiredScopes(field string) ([]string, bool) {
	rule, ok := p.Fields[field]
	if !ok {
		return nil, false
	}
	return rule.Scopes, true
}

// RoleResolver decides the caller's effective role for a given field. It
// receives the GraphQL field args so it can find a projectId for
// project-scoped operations. Implementations typically:
//
//   - return "owner" when the user's is_admin claim is true (system admin).
//   - look up Membership(projectId, userId) and return membership.role
//     when the field has a projectId arg.
//   - return ("", nil) when the caller has no project role and the field
//     doesn't need one.
//
// The string returned is fed back into Policy.MatchesAll, so it must be a
// role name the Catalog knows about (or "" for the no-role case).
type RoleResolver func(ctx context.Context, fieldArgs map[string]any) (role string, err error)
