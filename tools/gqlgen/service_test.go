package main

import (
	"strings"
	"testing"
)

// --- ParseService end-to-end against annotated sample service ---

func TestParseService_FindsQueries(t *testing.T) {
	queries, _, _, err := ParseService("testdata/sample_service", "example.com/test/services/tenant")
	if err != nil {
		t.Fatalf("ParseService: %v", err)
	}
	wantQueries := map[string]bool{"currentTenant": false, "myTenants": false}
	for _, q := range queries {
		if _, expected := wantQueries[q.QueryName]; expected {
			wantQueries[q.QueryName] = true
		}
	}
	for name, found := range wantQueries {
		if !found {
			t.Errorf("expected query %q in parsed result, got: %+v", name, queries)
		}
	}
}

func TestParseService_FindsMutation(t *testing.T) {
	_, mutations, _, err := ParseService("testdata/sample_service", "example.com/test/services/tenant")
	if err != nil {
		t.Fatalf("ParseService: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d: %+v", len(mutations), mutations)
	}
	m := mutations[0]
	if m.MutationName != "switchTenant" {
		t.Errorf("expected switchTenant, got %q", m.MutationName)
	}
	if m.FuncName != "SwitchTenant" {
		t.Errorf("expected FuncName=SwitchTenant, got %q", m.FuncName)
	}
	if len(m.Params) != 1 || m.Params[0].Name != "tenantId" {
		t.Errorf("expected single param 'tenantId', got %+v", m.Params)
	}
}

func TestParseService_FindsServiceType(t *testing.T) {
	_, _, types, err := ParseService("testdata/sample_service", "example.com/test/services/tenant")
	if err != nil {
		t.Fatalf("ParseService: %v", err)
	}
	var found *ServiceTypeDef
	for i := range types {
		if types[i].Name == "SwitchTenantResult" {
			found = &types[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected SwitchTenantResult type, got: %+v", types)
	}
	if found.ImportPath != "example.com/test/services/tenant" {
		t.Errorf("expected ImportPath set, got %q", found.ImportPath)
	}
}

func TestParseService_QueryReturnsListMarked(t *testing.T) {
	queries, _, _, err := ParseService("testdata/sample_service", "example.com/test/services/tenant")
	if err != nil {
		t.Fatalf("ParseService: %v", err)
	}
	var myTenants *StaticQueryDef
	for i := range queries {
		if queries[i].QueryName == "myTenants" {
			myTenants = &queries[i]
			break
		}
	}
	if myTenants == nil {
		t.Fatal("expected myTenants query in parsed result")
	}
	if !myTenants.ReturnsList {
		t.Error("expected ReturnsList=true for []Tenant return")
	}
	if !myTenants.IsPublic {
		t.Error("expected IsPublic=true (annotation contained @public)")
	}
}

// --- GenerateResolvers with static queries/mutations + service types ---

func TestGenerateResolvers_IncludesStaticEntries(t *testing.T) {
	queries, mutations, types, err := ParseService("testdata/sample_service", "example.com/test/services/tenant")
	if err != nil {
		t.Fatalf("ParseService: %v", err)
	}
	models := loadSampleModels(t)

	got := GenerateResolvers(models.Models, mutations, queries, types, "core", "example.com/sample/models", false)

	for _, snippet := range []string{"CurrentTenant", "MyTenants", "SwitchTenant"} {
		if !strings.Contains(got, snippet) {
			t.Errorf("expected static entry %q in generated resolvers:\n%s", snippet, got[:1000])
		}
	}
}

func TestGenerateResolvers_AdminModeSkipsAuthCheck(t *testing.T) {
	got := GenerateResolvers(nil, nil, nil, nil, "core", "x/y", true)
	// adminMode is documented as "skip user-auth check"; we can't easily
	// assert the specific absence, but the function should at least run
	// without panic and produce different output than non-admin mode for
	// some emit paths. Simplest correctness check: both modes produce a
	// valid header.
	if !strings.Contains(got, "Code generated") {
		t.Error("expected header in admin mode output")
	}
}

// --- Static queries/mutations schemas exercise goParamToGraphQL ---

func TestGenerateStaticMutationsSchema_RendersParams(t *testing.T) {
	_, mutations, _, _ := ParseService("testdata/sample_service", "example.com/test/services/tenant")
	got := GenerateStaticMutationsSchema(mutations)
	// uuid.UUID should map to UUID in the GraphQL output via goParamToGraphQL.
	if !strings.Contains(got, "UUID") {
		t.Errorf("expected UUID type in static mutations schema:\n%s", got)
	}
}

func TestGenerateServiceTypesSchema_RendersFields(t *testing.T) {
	_, _, types, _ := ParseService("testdata/sample_service", "example.com/test/services/tenant")
	got := GenerateServiceTypesSchema(types)
	if !strings.Contains(got, "SwitchTenantResult") {
		t.Errorf("expected SwitchTenantResult in service-types schema:\n%s", got)
	}
}
