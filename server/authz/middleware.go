package authz

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
)

// roleKey is the private context key used by AuthorizeField to stash the
// caller's resolved role so resolvers can use it for fine-grained logic
// (e.g. "admin can't remove other admins") without re-querying.
type contextKey string

const roleKey contextKey = "current_role"

// WithCurrentRole returns a context with the caller's resolved role
// attached. Called by AuthorizeField before invoking the resolver.
func WithCurrentRole(ctx context.Context, role string) context.Context {
	if role == "" {
		return ctx
	}
	return context.WithValue(ctx, roleKey, role)
}

// CurrentRole returns the caller's resolved role for the current field, or
// "" if no role was resolved (non-project field or unauthenticated).
func CurrentRole(ctx context.Context) string {
	v, _ := ctx.Value(roleKey).(string)
	return v
}

// AuthorizeField returns a gqlgen FieldMiddleware that enforces the
// consumer's EndpointPolicy against each top-level Query/Mutation field.
// Field-level (per-field) granularity means it runs for every resolved
// GraphQL field — so we explicitly gate on root-only fields (path length 1)
// and let nested type resolution flow through untouched.
//
// Wiring (in the consumer's main.go after building the gqlgen handler):
//
//	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
//	    Resolvers: &resolver.Resolver{DB: db.DB()},
//	}))
//	srv.AroundFields(authz.AuthorizeField(catalog, endpointPolicy, resolveRole))
//
// The middleware:
//
//  1. Passes through non-root fields immediately.
//  2. Allows public-listed fields with no further checks.
//  3. Resolves the caller's effective role via ResolveRole (e.g. is_admin →
//     "owner", or membership lookup for the projectId arg).
//  4. Looks up the field's required scopes in EndpointPolicy.
//  5. Asks Catalog whether the role's bundle satisfies them; on success
//     calls next, otherwise rejects with "missing scopes" error.
//
// Fields with no auth.yml entry are rejected with a configuration-error
// message. The scanner can layer in a build-time coverage check later;
// this runtime guard is belt-and-braces.
func AuthorizeField(catalog Policy, ep *EndpointPolicy, resolve RoleResolver) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		fc := graphql.GetFieldContext(ctx)
		if fc == nil {
			return next(ctx)
		}
		// Only authorize root operation fields. Nested fields on returned
		// objects (e.g. `project.name`) inherit the authorization decision
		// from their parent root call — no need to re-authorize them.
		if len(fc.Path()) != 1 {
			return next(ctx)
		}

		field := fc.Field.Name

		// GraphQL introspection fields (`__schema`, `__type`, `__typename`)
		// are spec-reserved and used by tooling (playgrounds, codegen,
		// schema browsers). Authorizing them would break every standard
		// GraphQL client. Always allow them through — the schema itself
		// is public information; sensitive ops behind it remain gated.
		if strings.HasPrefix(field, "__") {
			return next(ctx)
		}

		if ep.IsPublic(field) {
			return next(ctx)
		}

		scopes, ok := ep.RequiredScopes(field)
		if !ok {
			return nil, fmt.Errorf("authz: no policy entry for field %q — add it to auth.yml or list it under public:", field)
		}

		role := ""
		if resolve != nil {
			r, err := resolve(ctx, fc.Args)
			if err != nil {
				return nil, err
			}
			role = r
		}

		if !catalog.MatchesAll(role, scopes) {
			return nil, fmt.Errorf("authz: caller lacks permission for %s (required scopes: %v)", field, scopes)
		}

		// Stash the resolved role for the resolver — fine-grained checks
		// inside the resolver body (e.g. role-vs-role logic in removeMember)
		// can read it via authz.CurrentRole(ctx) without re-querying.
		return next(WithCurrentRole(ctx, role))
	}
}
