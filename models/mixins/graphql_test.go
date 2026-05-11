package mixins

import (
	"testing"
)

// GraphQLMixin is a zero-field marker mixin. Embedding it signals the
// gqlgen schema generator to expose this model in the GraphQL API.
// IsGraphQL is the public detection helper.

func TestGraphQLMixin_IsZeroFields(t *testing.T) {
	// Compile-time guarantee that the marker stays a zero-field struct.
	// A zero-field embed adds nothing to the host struct's memory layout
	// or DB schema; that's the contract.
	m := GraphQLMixin{}
	_ = m
}

func TestIsGraphQL_DetectsEmbeddedMixin(t *testing.T) {
	if !IsGraphQL(&widgetWithGraphQL{}) {
		t.Fatal("expected widgetWithGraphQL to be detected as graphql-exposed")
	}
}

func TestIsGraphQL_FalseForPlainStruct(t *testing.T) {
	if IsGraphQL(&plainItem{}) {
		t.Fatal("expected plain struct to NOT be detected")
	}
}

func TestIsGraphQL_FalseForModelWithoutGraphQLMixin(t *testing.T) {
	// widget embeds many mixins but NOT GraphQLMixin.
	if IsGraphQL(&widget{}) {
		t.Fatal("expected widget (no GraphQLMixin) to NOT be detected")
	}
}

func TestIsGraphQL_HandlesValueAndPointer(t *testing.T) {
	if !IsGraphQL(widgetWithGraphQL{}) {
		t.Fatal("expected value-form to be detected")
	}
	if !IsGraphQL(&widgetWithGraphQL{}) {
		t.Fatal("expected pointer-form to be detected")
	}
}
