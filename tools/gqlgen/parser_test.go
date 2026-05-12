package main

import (
	"strings"
	"testing"
)

// modelByName looks up a parsed model by its Go struct name.
func modelByName(result *ParseResult, name string) *ModelDef {
	for i := range result.Models {
		if result.Models[i].Name == name {
			return &result.Models[i]
		}
	}
	return nil
}

// --- ParseModels happy path ---

func TestParseModels_FindsGraphQLMixinModels(t *testing.T) {
	result, err := ParseModels("testdata/sample_models")
	if err != nil {
		t.Fatalf("ParseModels: %v", err)
	}

	// Should find Agent, User, AgentVersion. NotAModel must be excluded
	// (no GraphQLMixin).
	wantNames := map[string]bool{"Agent": false, "User": false, "AgentVersion": false}
	for _, m := range result.Models {
		if _, expected := wantNames[m.Name]; expected {
			wantNames[m.Name] = true
		}
		if m.Name == "NotAModel" {
			t.Errorf("NotAModel must be excluded (no GraphQLMixin), but was included")
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("expected to find model %q in ParseResult", name)
		}
	}
}

func TestParseModels_AgentFieldsExtracted(t *testing.T) {
	result, err := ParseModels("testdata/sample_models")
	if err != nil {
		t.Fatalf("ParseModels: %v", err)
	}
	agent := modelByName(result, "Agent")
	if agent == nil {
		t.Fatal("expected Agent model in result")
	}

	// Direct fields should include AgentID (primary key), Name, Slug, Description, OwnerID.
	// Mixin-contributed fields (CreatedAt, UpdatedAt, TenantID, etc.) should also appear.
	wantFields := map[string]bool{
		"AgentID":     false,
		"Name":        false,
		"Slug":        false,
		"Description": false,
		"OwnerID":     false,
		"CreatedAt":   false, // from BaseModel mixin
		"UpdatedAt":   false, // from BaseModel mixin
		"TenantID":    false, // from TenantMixin
		"CreatedBy":   false, // from TrackedMixin
		"UpdatedBy":   false, // from TrackedMixin
	}
	for _, f := range agent.Fields {
		if _, expected := wantFields[f.GoName]; expected {
			wantFields[f.GoName] = true
		}
	}
	for name, found := range wantFields {
		if !found {
			t.Errorf("expected Agent to have field %q, fields: %+v", name, fieldNames(agent.Fields))
		}
	}
}

func TestParseModels_TagsPreservedOnFields(t *testing.T) {
	// Parser keeps every field including those with graphql:"-" — generators
	// filter on the tag downstream. Verify the tag is preserved.
	result, _ := ParseModels("testdata/sample_models")
	agent := modelByName(result, "Agent")
	if agent == nil {
		t.Fatal("expected Agent model")
	}
	var hidden, readonly *FieldDef
	for i := range agent.Fields {
		switch agent.Fields[i].GoName {
		case "Hidden":
			hidden = &agent.Fields[i]
		case "ReadOnly":
			readonly = &agent.Fields[i]
		}
	}
	if hidden == nil || hidden.GraphQLTag != "-" {
		t.Errorf("expected Hidden field with GraphQLTag='-', got %+v", hidden)
	}
	if readonly == nil || readonly.GraphQLTag != "readonly" {
		t.Errorf("expected ReadOnly field with GraphQLTag='readonly', got %+v", readonly)
	}
}

func TestParseModels_PrimaryKeyMarked(t *testing.T) {
	result, _ := ParseModels("testdata/sample_models")
	agent := modelByName(result, "Agent")
	if agent == nil {
		t.Fatal("expected Agent model")
	}
	var pk *FieldDef
	for i := range agent.Fields {
		if agent.Fields[i].IsPrimaryKey {
			pk = &agent.Fields[i]
			break
		}
	}
	if pk == nil {
		t.Fatal("expected Agent to have a primary key field marked")
	}
	if pk.GoName != "AgentID" {
		t.Errorf("expected AgentID as primary key, got %q", pk.GoName)
	}
}

func TestParseModels_RelationshipsExtracted(t *testing.T) {
	result, _ := ParseModels("testdata/sample_models")
	agent := modelByName(result, "Agent")
	if agent == nil {
		t.Fatal("expected Agent model")
	}
	wantRels := map[string]bool{"Owner": false, "Versions": false}
	for _, r := range agent.Relationships {
		if _, expected := wantRels[r.GoName]; expected {
			wantRels[r.GoName] = true
		}
	}
	for name, found := range wantRels {
		if !found {
			t.Errorf("expected relationship %q, got: %+v", name, agent.Relationships)
		}
	}
}

func TestParseModels_DescriptionExtracted(t *testing.T) {
	result, _ := ParseModels("testdata/sample_models")
	agent := modelByName(result, "Agent")
	if agent == nil {
		t.Fatal("expected Agent model")
	}
	if !strings.Contains(agent.Description, "registered AI agent") {
		t.Errorf("expected description to contain 'registered AI agent', got %q", agent.Description)
	}
}

func TestParseModels_NonexistentDirReturnsError(t *testing.T) {
	_, err := ParseModels("testdata/this_does_not_exist")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

// --- ParseService happy path ---

func TestParseService_NonexistentDirReturnsError(t *testing.T) {
	_, _, _, err := ParseService("testdata/this_does_not_exist", "example.com/test")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestParseService_EmptyDirReturnsNoStatics(t *testing.T) {
	// testdata/sample_models has no @gql annotations on functions — only
	// model types. ParseService should walk it without producing static
	// queries or mutations.
	queries, mutations, types, err := ParseService("testdata/sample_models", "example.com/test")
	if err != nil {
		t.Fatalf("ParseService: %v", err)
	}
	if len(queries) != 0 {
		t.Errorf("expected no queries, got %+v", queries)
	}
	if len(mutations) != 0 {
		t.Errorf("expected no mutations, got %+v", mutations)
	}
	// types may still be non-empty if any structs are defined — fine, no assertion.
	_ = types
}

// helper
func fieldNames(fields []FieldDef) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.GoName)
	}
	return out
}
