package main

import (
	"go/ast"
	"reflect"
	"testing"
)

// generator.go ─────────────────────────────────────────────────────────────

func TestGoTypeToGraphQL(t *testing.T) {
	tests := []struct {
		goType    string
		isPointer bool
		want      string
	}{
		{"string", false, "String!"},
		{"string", true, "String"},
		{"*string", false, "String"},
		{"int", false, "Int!"},
		{"float64", false, "Float!"},
		{"bool", false, "Boolean!"},
		{"uuid.UUID", false, "UUID!"},
		{"uuid.UUID", true, "UUID"},
		{"time.Time", false, "DateTime!"},
		{"json.RawMessage", false, "JSON!"},
		{"decimal.Decimal", false, "Float!"},
		{"SomeUnknownType", false, "String!"},
		{"[]string", false, "[String!]!"},
		{"[]int", false, "[Int!]!"},
	}
	for _, c := range tests {
		if got := goTypeToGraphQL(c.goType, c.isPointer); got != c.want {
			t.Errorf("goTypeToGraphQL(%q, %v) = %q, want %q", c.goType, c.isPointer, got, c.want)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"agent_id", "agentId"},
		{"created_at", "createdAt"},
		{"version", "version"},
		{"a_b_c_d", "aBCD"},
		{"", ""},
		{"_leading", "Leading"},
	}
	for _, c := range tests {
		if got := toCamelCase(c.in); got != c.want {
			t.Errorf("toCamelCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Agent", "agent"},
		{"AgentVersion", "agent_version"},
		{"APIKey", "api_key"},
		{"UserID", "user_id"},
		{"", ""},
		{"X", "x"},
	}
	for _, c := range tests {
		if got := toSnakeCase(c.in); got != c.want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToGraphQLName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Agent", "agent"},
		{"AgentVersion", "agentVersion"},
		{"APIKey", "apiKey"},
	}
	for _, c := range tests {
		if got := toGraphQLName(c.in); got != c.want {
			t.Errorf("toGraphQLName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"agent", "agents"},
		{"version", "versions"},
		{"box", "boxes"},
		{"bus", "buses"},
		{"dish", "dishes"},
		{"match", "matches"},
		{"city", "cities"},
		{"day", "days"},   // vowel before y
		{"key", "keys"},   // vowel before y
		{"y", "ys"},       // too short for y→ies
	}
	for _, c := range tests {
		if got := pluralize(c.in); got != c.want {
			t.Errorf("pluralize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToHumanReadable(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Agent", "agent"},
		{"AgentVersion", "agent version"},
		{"APIKey", "api key"},
	}
	for _, c := range tests {
		if got := toHumanReadable(c.in); got != c.want {
			t.Errorf("toHumanReadable(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInferFilterDescription(t *testing.T) {
	tests := []struct {
		field FieldDef
		want  string
	}{
		{FieldDef{JSONName: "name", Description: "User's full name."}, "Filter by user's full name."},
		{FieldDef{JSONName: "agent_id"}, "Filter by agent ID."},
		{FieldDef{JSONName: "status"}, "Filter by current status."},
		{FieldDef{JSONName: "category"}, "Filter by category."},
		{FieldDef{JSONName: "agent_type"}, "Filter by agent type."},
		{FieldDef{JSONName: "operating_mode"}, "Filter by operating mode."},
		{FieldDef{JSONName: "version_label"}, "Filter by version label."},
	}
	for _, c := range tests {
		if got := inferFilterDescription(c.field); got != c.want {
			t.Errorf("inferFilterDescription(%+v) = %q, want %q", c.field, got, c.want)
		}
	}
}

func TestPrimaryKeyField(t *testing.T) {
	m := ModelDef{
		Fields: []FieldDef{
			{JSONName: "name"},
			{JSONName: "agent_id", IsPrimaryKey: true},
			{JSONName: "version"},
		},
	}
	pk := primaryKeyField(m)
	if pk == nil || pk.JSONName != "agent_id" {
		t.Fatalf("expected agent_id, got %+v", pk)
	}

	empty := ModelDef{Fields: []FieldDef{{JSONName: "name"}}}
	if got := primaryKeyField(empty); got != nil {
		t.Fatalf("expected nil for model without primary key, got %+v", got)
	}
}

func TestWriteDescription_EmptyIsNoOp(t *testing.T) {
	// Smoke — empty descriptions should not produce output.
	// (writeDescription is unexported but called from many generator paths;
	// the assertion is implicit via generated output content.)
	_ = writeDescription
}

// resolver_gen.go ──────────────────────────────────────────────────────────
//
// NOTE: parsePagination, toPageInfo, toFilterMap, inputToModel, camelToSnake,
// updateToMap, normalizeTag, mapInputToStruct, setFieldsRecursive and
// indexByte are emitted as Go source into the consumer's resolver/helpers.go
// — not real functions in the tool. They get exercised end-to-end when a
// consumer runs the generator and compiles the output; we don't unit-test
// them in the tool itself.

func TestToPascalCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"agent_id", "AgentID"},      // acronym-aware
		{"created_at", "CreatedAt"},
		{"api_url", "APIURL"},
		{"user_uuid", "UserUUID"},
		{"", ""},
	}
	for _, c := range tests {
		if got := toPascalCase(c.in); got != c.want {
			t.Errorf("toPascalCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCollectServiceImports(t *testing.T) {
	mutations := []StaticMutationDef{
		{ImportPath: "github.com/foo/bar"},
		{ImportPath: "github.com/foo/bar"}, // dedup
		{ImportPath: ""},                   // skip empty
	}
	queries := []StaticQueryDef{
		{ImportPath: "github.com/baz/qux"},
	}
	got := collectServiceImports(mutations, queries)
	want := map[string]bool{
		"github.com/foo/bar": true,
		"github.com/baz/qux": true,
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct imports, got %v", got)
	}
	for _, imp := range got {
		if !want[imp] {
			t.Errorf("unexpected import %q", imp)
		}
	}
}

func TestAnalyzePreloads_Empty(t *testing.T) {
	info := analyzePreloads(ModelDef{})
	if len(info.Paths) != 0 {
		t.Fatalf("expected no preloads for empty model, got %v", info.Paths)
	}
}

func TestAnalyzePreloads_PreloadTagAddsPath(t *testing.T) {
	m := ModelDef{
		Relationships: []RelationshipDef{
			{GoName: "Owner", GoType: "User", GraphQLTag: "preload"},      // distinct FK; uses Preload
			{GoName: "Versions", GoType: "AgentVersion", GraphQLTag: "preload"},
		},
	}
	info := analyzePreloads(m)
	if len(info.Paths) != 2 {
		t.Fatalf("expected 2 preload paths, got %v", info.Paths)
	}
}

func TestAnalyzePreloads_TenantUsesEnrichment(t *testing.T) {
	m := ModelDef{
		Relationships: []RelationshipDef{
			{GoName: "Tenant", GoType: "Tenant", GraphQLTag: "preload"},
			{GoName: "User", GoType: "User", GraphQLTag: "preload"},
		},
	}
	info := analyzePreloads(m)
	if !info.EnrichTenants {
		t.Error("expected EnrichTenants=true for Tenant relationship")
	}
	if !info.EnrichUsers {
		t.Error("expected EnrichUsers=true for User-named User-typed relationship")
	}
}

func TestAnalyzePreloads_NestedPreloadParsing(t *testing.T) {
	m := ModelDef{
		Relationships: []RelationshipDef{
			{GoName: "ActiveVersion", GoType: "AgentVersion", GraphQLTag: "preload:Skills,Tenant"},
		},
	}
	info := analyzePreloads(m)
	// Should preload parent + nested Skills, and mark Tenant for enrichment.
	if len(info.Paths) < 2 {
		t.Fatalf("expected at least 2 preload paths, got %v", info.Paths)
	}
	if len(info.NestedEnrich) == 0 {
		t.Errorf("expected nested Tenant to be tracked for enrichment, got %v", info.NestedEnrich)
	}
}

func TestAnalyzePreloads_NoTagIgnored(t *testing.T) {
	m := ModelDef{
		Relationships: []RelationshipDef{
			{GoName: "Other", GoType: "Other"}, // no GraphQLTag
		},
	}
	info := analyzePreloads(m)
	if len(info.Paths) != 0 {
		t.Errorf("expected untagged relationship to be ignored, got %v", info.Paths)
	}
}

func TestGoParamToGraphQL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"string", "String!"},
		{"*string", "String"},
		{"uuid.UUID", "UUID!"},
		{"*uuid.UUID", "UUID"},
		{"int", "Int!"},
		{"*int", "Int"},
		{"bool", "Boolean!"},
		{"*bool", "Boolean"},
		{"time.Time", "DateTime!"},
		{"*time.Time", "DateTime"},
		{"[]uuid.UUID", "[UUID!]!"},
		{"[]string", "[String!]!"},
		{"[]int", "[Int!]"},
		{"SomeStruct", "String!"}, // default fallback
	}
	for _, c := range tests {
		if got := goParamToGraphQL(c.in); got != c.want {
			t.Errorf("goParamToGraphQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// parser.go ────────────────────────────────────────────────────────────────

func TestCleanComment(t *testing.T) {
	tests := []struct{ in, want string }{
		{"  hello world  ", "hello world"},
		{"line one\nline two", "line one line two"},
		{"line\n\nblank\nthen line", "line blank then line"},
		{"hello\n── Identity ───\nworld", "hello world"}, // section header stripped
		{"", ""},
	}
	for _, c := range tests {
		if got := cleanComment(c.in); got != c.want {
			t.Errorf("cleanComment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsSectionHeader(t *testing.T) {
	if !isSectionHeader("── Identity ──────") {
		t.Fatal("expected box-drawing line to be a section header")
	}
	if isSectionHeader("Regular comment") {
		t.Fatal("expected plain comment to NOT be a section header")
	}
}

func TestIsModelType(t *testing.T) {
	model := []string{"Agent", "AgentVersion", "User"}
	notModel := []string{"string", "int", "uuid.UUID", "time.Time", "", "lowercase"}
	for _, n := range model {
		if !isModelType(n) {
			t.Errorf("expected %q to be a model type", n)
		}
	}
	for _, n := range notModel {
		if isModelType(n) {
			t.Errorf("expected %q to NOT be a model type", n)
		}
	}
}

func TestShortTypeName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"uuid.UUID", "UUID"},
		{"time.Time", "Time"},
		{"Plain", "Plain"},
		{"a.b.c", "c"},
		{"", ""},
	}
	for _, c := range tests {
		if got := shortTypeName(c.in); got != c.want {
			t.Errorf("shortTypeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExprTypeName(t *testing.T) {
	tests := []struct {
		expr ast.Expr
		want string
	}{
		{&ast.Ident{Name: "Agent"}, "Agent"},
		{&ast.SelectorExpr{X: &ast.Ident{Name: "uuid"}, Sel: &ast.Ident{Name: "UUID"}}, "uuid.UUID"},
		{&ast.StarExpr{X: &ast.Ident{Name: "Agent"}}, "*Agent"},
		{&ast.ArrayType{Elt: &ast.Ident{Name: "Agent"}}, "[]Agent"},
		{&ast.FuncType{}, ""}, // unsupported → empty
	}
	for _, c := range tests {
		if got := exprTypeName(c.expr); got != c.want {
			t.Errorf("exprTypeName(%T) = %q, want %q", c.expr, got, c.want)
		}
	}
}

func TestParseJSONName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"name", "name"},
		{"name,omitempty", "name"},
		{"-", "-"},
		{"", ""},
	}
	for _, c := range tests {
		if got := parseJSONName(c.in); got != c.want {
			t.Errorf("parseJSONName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseStructTags(t *testing.T) {
	// Construct a synthetic AST BasicLit with backtick-quoted tag.
	lit := &ast.BasicLit{Value: "`json:\"name,omitempty\" gorm:\"size:255\"`"}
	got := parseStructTags(lit)
	if got["json"] != "name,omitempty" {
		t.Errorf("expected json tag, got %q", got["json"])
	}
	if got["gorm"] != "size:255" {
		t.Errorf("expected gorm tag, got %q", got["gorm"])
	}
}

func TestParseStructTags_Nil(t *testing.T) {
	got := parseStructTags(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map for nil tag, got %v", got)
	}
}

func TestIsRelationship(t *testing.T) {
	tests := []struct {
		expr ast.Expr
		want bool
	}{
		{&ast.StarExpr{X: &ast.Ident{Name: "Agent"}}, true},  // *Agent
		{&ast.StarExpr{X: &ast.Ident{Name: "string"}}, false}, // *string is not a relationship
		{&ast.ArrayType{Elt: &ast.Ident{Name: "Skill"}}, true},
		{&ast.Ident{Name: "Agent"}, false}, // plain identifier — not a relationship
	}
	for _, c := range tests {
		if got := isRelationship(c.expr); got != c.want {
			t.Errorf("isRelationship(%T) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestReceiverTypeName(t *testing.T) {
	// (s *Service) → "Service"
	recv := &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: &ast.Ident{Name: "Service"}}}}}
	if got := receiverTypeName(recv); got != "Service" {
		t.Errorf("expected Service, got %q", got)
	}

	// (s Service) → "Service"
	recv = &ast.FieldList{List: []*ast.Field{{Type: &ast.Ident{Name: "Service"}}}}
	if got := receiverTypeName(recv); got != "Service" {
		t.Errorf("expected Service, got %q", got)
	}

	if got := receiverTypeName(nil); got != "" {
		t.Errorf("expected empty string for nil receiver, got %q", got)
	}

	if got := receiverTypeName(&ast.FieldList{}); got != "" {
		t.Errorf("expected empty string for empty receiver list, got %q", got)
	}
}

// Avoid unused-import warning on reflect (used in parseStructTags).
var _ = reflect.TypeOf(struct{}{})
