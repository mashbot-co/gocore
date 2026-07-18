package main

import (
	"strings"
	"testing"
)

// loadSampleModels parses the testdata sample_models directory and returns
// the result. Used by both generator and resolver_gen tests.
func loadSampleModels(t *testing.T) *ParseResult {
	t.Helper()
	result, err := ParseModels("testdata/sample_models")
	if err != nil {
		t.Fatalf("ParseModels: %v", err)
	}
	return result
}

// --- GenerateSchema produces all expected files ---

func TestGenerateSchema_EmitsAllExpectedFiles(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)

	expectedFiles := []string{
		"types.graphql",
		"inputs.graphql",
		"queries.graphql",
		"mutations.graphql",
		"custom_mutations.graphql",
		"connections.graphql",
	}
	for _, name := range expectedFiles {
		content, ok := files[name]
		if !ok {
			t.Errorf("expected %q in generated files", name)
			continue
		}
		if content == "" {
			t.Errorf("expected non-empty content for %q", name)
		}
	}
}

func TestGenerateSchema_TypesIncludeAgent(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)
	types := files["types.graphql"]

	if !strings.Contains(types, "type Agent {") {
		t.Errorf("expected 'type Agent {' in types.graphql:\n%s", types)
	}
	if !strings.Contains(types, "agentId: UUID!") {
		t.Errorf("expected 'agentId: UUID!' field, got:\n%s", types)
	}
}

func TestGenerateSchema_InputsIncludeCreateAndUpdate(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)
	inputs := files["inputs.graphql"]

	if !strings.Contains(inputs, "input CreateAgentInput") {
		t.Errorf("expected CreateAgentInput in inputs.graphql:\n%s", inputs)
	}
	if !strings.Contains(inputs, "input UpdateAgentInput") {
		t.Errorf("expected UpdateAgentInput in inputs.graphql:\n%s", inputs)
	}
}

func TestGenerateSchema_QueriesIncludeSingularAndListForms(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)
	queries := files["queries.graphql"]

	if !strings.Contains(queries, "agent(agentId: UUID!): Agent") {
		t.Errorf("expected agent(agentId: UUID!) query, got:\n%s", queries)
	}
	if !strings.Contains(queries, "agents(filter:") {
		t.Errorf("expected list query 'agents(filter: ...)', got:\n%s", queries)
	}
}

func TestGenerateSchema_MutationsIncludeCRUD(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)
	mutations := files["mutations.graphql"]

	for _, snippet := range []string{"createAgent", "updateAgent", "deleteAgent"} {
		if !strings.Contains(mutations, snippet) {
			t.Errorf("expected %q in mutations.graphql:\n%s", snippet, mutations)
		}
	}
}

// --- Hidden fields excluded from emitted schema ---

func TestGenerateSchema_HiddenFieldsExcluded(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)
	types := files["types.graphql"]
	if strings.Contains(types, "hidden:") {
		t.Errorf("graphql:\"-\" field 'Hidden' should NOT appear in types.graphql:\n%s", types)
	}
}

// --- Comma-separated graphql tags (e.g. "readonly,nullable") ---

func TestGenerateSchema_CommaTagReadonlyExcludedFromInputs(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)
	inputs := files["inputs.graphql"]

	if strings.Contains(inputs, "payload:") {
		t.Errorf("graphql:\"readonly,nullable\" field 'Payload' should NOT appear in inputs.graphql:\n%s", inputs)
	}
}

func TestGenerateSchema_CommaTagNullableEmitsOptionalType(t *testing.T) {
	result := loadSampleModels(t)
	files := GenerateSchema(result.Models)
	types := files["types.graphql"]

	if !strings.Contains(types, "payload: JSON\n") {
		t.Errorf("expected nullable 'payload: JSON' in types.graphql:\n%s", types)
	}
	if strings.Contains(types, "payload: JSON!") {
		t.Errorf("graphql:\"readonly,nullable\" field 'Payload' must not be non-null:\n%s", types)
	}
}

// --- Bindings ---

func TestGenerateBindings_DocumentsEveryModel(t *testing.T) {
	result := loadSampleModels(t)
	bindings := GenerateBindings(result.Models, "example.com/sample/models")

	for _, name := range []string{"Agent:", "User:", "AgentVersion:"} {
		if !strings.Contains(bindings, name) {
			t.Errorf("expected %q in bindings output:\n%s", name, bindings)
		}
	}
	if !strings.Contains(bindings, "example.com/sample/models.Agent") {
		t.Errorf("expected consumer's models import path in bindings, got:\n%s", bindings)
	}
}

// --- Static queries / mutations / service-type schemas ---

func TestGenerateStaticQueriesSchema_EmptyInput(t *testing.T) {
	got := GenerateStaticQueriesSchema(nil)
	if got == "" {
		t.Fatal("expected non-empty output even for empty input (file header)")
	}
}

func TestGenerateStaticMutationsSchema_EmptyInput(t *testing.T) {
	got := GenerateStaticMutationsSchema(nil)
	if got == "" {
		t.Fatal("expected non-empty output even for empty input (file header)")
	}
}

func TestGenerateServiceTypesSchema_EmptyInput(t *testing.T) {
	got := GenerateServiceTypesSchema(nil)
	if got == "" {
		t.Fatal("expected non-empty output even for empty input (file header)")
	}
}

func TestGenerateStaticQueriesSchema_RendersQueryEntry(t *testing.T) {
	queries := []StaticQueryDef{
		{
			QueryName:   "currentUser",
			FuncName:    "GetCurrentUser",
			Description: "Returns the currently authenticated user.",
			ReturnType:  "User",
		},
	}
	got := GenerateStaticQueriesSchema(queries)
	if !strings.Contains(got, "currentUser") {
		t.Errorf("expected currentUser in static queries schema:\n%s", got)
	}
}

func TestGenerateStaticMutationsSchema_RendersMutationEntry(t *testing.T) {
	mutations := []StaticMutationDef{
		{
			MutationName: "lockScope",
			FuncName:     "LockScope",
			Description:  "Locks a scope.",
			ReturnType:   "Scope",
			Params: []ParamDef{
				{Name: "scopeId", GoType: "uuid.UUID"},
			},
		},
	}
	got := GenerateStaticMutationsSchema(mutations)
	if !strings.Contains(got, "lockScope") {
		t.Errorf("expected lockScope in static mutations schema:\n%s", got)
	}
}
