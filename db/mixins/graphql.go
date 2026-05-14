package mixins

// GraphQLMixin is a marker mixin. Embed it in a model to include
// that model in the auto-generated GraphQL schema.
//
// The schema generator (tools/gqlgen-schema) scans all models for
// this mixin and produces .graphql type definitions, queries,
// mutations, and input types automatically.
//
// Use the `graphql` struct tag on fields to control schema generation:
//
//   - graphql:"-"          Exclude from GraphQL schema entirely
//   - graphql:"readonly"   Include in type but not in create/update inputs
//   - graphql:"filterable" Include in filter input for list queries
//   - (no tag)             Included in type and inputs by default
type GraphQLMixin struct{}

// IsGraphQL checks whether a model embeds GraphQLMixin.
func IsGraphQL(model interface{}) bool {
	return hasEmbeddedField(model, "GraphQLMixin")
}
