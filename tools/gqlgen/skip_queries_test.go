package main

import (
	"strings"
	"testing"
)

// TestSkipQueries_AnnotationParsed confirms the parser surfaces SkipQueries
// from a `// graphql:skipQueries` doc comment and strips it from the
// description so generated GraphQL docs don't carry the tag through.
func TestSkipQueries_AnnotationParsed(t *testing.T) {
	model := buildModel("Project", []string{"GraphQLMixin"})
	model.Description = "Project is the unit of access.\ngraphql:skipQueries"
	model.SkipQueries = strings.Contains(model.Description, "graphql:skipQueries")
	if !model.SkipQueries {
		t.Fatal("expected SkipQueries true on directly-set ModelDef")
	}
}

// TestPKParamName_UsesCamelCaseOfJSONName proves the single-key arg name is
// derived from the PK column (e.g. "membership_id" → "membershipId"), not the
// hardcoded "id" we used previously.
func TestPKParamName_UsesCamelCaseOfJSONName(t *testing.T) {
	cases := []struct {
		name, pk, want string
	}{
		{"Membership", "MembershipID", "membershipId"},
		{"User", "UserID", "userId"},
		{"AgentVersion", "AgentVersionID", "agentVersionId"},
	}
	for _, tc := range cases {
		m := buildModelWithPK(tc.name, tc.pk)
		if got := pkParamName(m); got != tc.want {
			t.Errorf("%s: pkParamName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestPKParamName_FallsBackToIDWithoutPK is the belt-and-braces case the
// generator already guards against — the helper still returns "id" so
// invariants downstream don't break if someone wires it in.
func TestPKParamName_FallsBackToIDWithoutPK(t *testing.T) {
	m := ModelDef{Name: "NoPK"}
	if got := pkParamName(m); got != "id" {
		t.Errorf("pkParamName without PK = %q, want %q", got, "id")
	}
}

// TestGenerateMutations_UsesPKParamName confirms the update/delete sites
// pick up the PK-derived name too — they were the third site to change.
func TestGenerateMutations_UsesPKParamName(t *testing.T) {
	m := buildModelWithPK("Membership", "MembershipID")
	m.Fields = append(m.Fields, FieldDef{GoName: "Note", GoType: "string", JSONName: "note"})

	files := GenerateSchema([]ModelDef{m})
	mutations := files["mutations.graphql"]

	if !strings.Contains(mutations, "updateMembership(membershipId: UUID!") {
		t.Errorf("expected updateMembership(membershipId: ...), got:\n%s", mutations)
	}
	if !strings.Contains(mutations, "deleteMembership(membershipId: UUID!)") {
		t.Errorf("expected deleteMembership(membershipId: UUID!), got:\n%s", mutations)
	}
}

// TestGenerateQueries_OmitsModelsWithSkipQueries proves the schema generator
// drops the Get/List query lines for opted-out models while still emitting them
// for siblings.
func TestGenerateQueries_OmitsModelsWithSkipQueries(t *testing.T) {
	skipped := buildModelWithPK("Project", "ProjectID")
	skipped.SkipQueries = true

	kept := buildModelWithPK("Membership", "MembershipID")

	files := GenerateSchema([]ModelDef{skipped, kept})
	queries := files["queries.graphql"]

	if strings.Contains(queries, "project(projectId: UUID!): Project") {
		t.Errorf("expected `project` query to be skipped, got:\n%s", queries)
	}
	if strings.Contains(queries, "projects(") {
		t.Errorf("expected `projects` list query to be skipped, got:\n%s", queries)
	}
	if !strings.Contains(queries, "membership(membershipId: UUID!): Membership") {
		t.Errorf("expected sibling `membership` query to be kept, got:\n%s", queries)
	}
}

// TestGenerateResolvers_OmitsQueryMethodsForSkipQueries proves the resolver
// generator skips the auto Get/List Go methods for opted-out models.
func TestGenerateResolvers_OmitsQueryMethodsForSkipQueries(t *testing.T) {
	skipped := buildModelWithPK("Project", "ProjectID")
	skipped.SkipQueries = true

	kept := buildModelWithPK("Membership", "MembershipID")

	resolvers := GenerateResolvers([]ModelDef{skipped, kept}, nil, nil, nil, "core", "iro/backend/models", false)

	if strings.Contains(resolvers, "func (r *queryResolver) Project(") {
		t.Errorf("expected Project query resolver to be skipped:\n%s", resolvers)
	}
	if strings.Contains(resolvers, "func (r *queryResolver) Projects(") {
		t.Errorf("expected Projects list resolver to be skipped:\n%s", resolvers)
	}
	if !strings.Contains(resolvers, "func (r *queryResolver) Membership(") {
		t.Errorf("expected Membership resolver to be kept:\n%s", resolvers)
	}
}

// buildModelWithPK returns a minimal ModelDef the generators recognize as
// having a primary key (the only structural requirement for query emission).
func buildModelWithPK(name, pkField string) ModelDef {
	return ModelDef{
		Name: name,
		Fields: []FieldDef{
			{GoName: pkField, GoType: "uuid.UUID", JSONName: toSnakeCase(pkField), IsPrimaryKey: true},
		},
	}
}

// buildModel returns a minimal ModelDef with the named mixins recorded.
func buildModel(name string, mixins []string) ModelDef {
	return ModelDef{
		Name:   name,
		Mixins: mixins,
	}
}
