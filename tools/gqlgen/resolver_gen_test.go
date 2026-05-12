package main

import (
	"strings"
	"testing"
)

// --- GenerateResolvers end-to-end via testdata models ---

func TestGenerateResolvers_EmitsCorePackageHeader(t *testing.T) {
	result := loadSampleModels(t)
	got := GenerateResolvers(result.Models, nil, nil, nil, "core", "example.com/sample/models", false)

	if !strings.Contains(got, "package resolver") {
		t.Errorf("expected 'package resolver' in output:\n%s", got)
	}
	if !strings.Contains(got, "Code generated") {
		t.Error("expected 'Code generated' header")
	}
}

func TestGenerateResolvers_ImportsModelsAtConsumerPath(t *testing.T) {
	result := loadSampleModels(t)
	got := GenerateResolvers(result.Models, nil, nil, nil, "core", "example.com/sample/models", false)

	if !strings.Contains(got, `models "example.com/sample/models"`) {
		t.Errorf("expected models import at consumer's path, got:\n%s", got)
	}
	if !strings.Contains(got, `"github.com/mashbot-co/gocore/connection"`) {
		t.Errorf("expected connection import at gocore's canonical path, got:\n%s", got)
	}
}

func TestGenerateResolvers_EmitsQueryResolversForEveryModel(t *testing.T) {
	result := loadSampleModels(t)
	got := GenerateResolvers(result.Models, nil, nil, nil, "core", "example.com/sample/models", false)

	for _, name := range []string{"Agent", "User", "AgentVersion"} {
		if !strings.Contains(got, "func (r *queryResolver) "+name+"(") {
			t.Errorf("expected singular query resolver for %s:\n%s", name, got)
		}
		if !strings.Contains(got, "func (r *queryResolver) "+pluralize(name)+"(") {
			t.Errorf("expected list query resolver for %s:\n%s", name, got)
		}
	}
}

func TestGenerateResolvers_EmitsMutationResolversForEveryModel(t *testing.T) {
	result := loadSampleModels(t)
	got := GenerateResolvers(result.Models, nil, nil, nil, "core", "example.com/sample/models", false)

	for _, prefix := range []string{"Create", "Update", "Delete"} {
		if !strings.Contains(got, prefix+"Agent") {
			t.Errorf("expected %sAgent mutation resolver, got:\n%s", prefix, got)
		}
	}
}

func TestGenerateResolvers_ModuleNameDefault(t *testing.T) {
	// Empty moduleName should default to "core" — verify this by checking
	// the package path of generated.Config which the resolver wires up.
	got := GenerateResolvers(nil, nil, nil, nil, "", "x/y/models", false)
	if !strings.Contains(got, `"core/graph/generated"`) {
		t.Errorf("expected default 'core' moduleName in generated/import:\n%s", got)
	}
}

func TestGenerateResolvers_CustomModuleName(t *testing.T) {
	got := GenerateResolvers(nil, nil, nil, nil, "myapi", "x/y/models", false)
	if !strings.Contains(got, `"myapi/graph/generated"`) {
		t.Errorf("expected 'myapi/graph/generated' import, got:\n%s", got)
	}
}

// --- GenerateResolverHelpers emits a syntactically reasonable file ---

func TestGenerateResolverHelpers_HasPackageAndExpectedFunctions(t *testing.T) {
	got := GenerateResolverHelpers(nil, "myapi")
	if !strings.Contains(got, "package resolver") {
		t.Error("expected 'package resolver'")
	}
	if !strings.Contains(got, "func parsePagination") {
		t.Error("expected parsePagination emitted")
	}
	if !strings.Contains(got, "func toFilterMap") {
		t.Error("expected toFilterMap emitted")
	}
	if !strings.Contains(got, "func toPageInfo") {
		t.Error("expected toPageInfo emitted")
	}
	if !strings.Contains(got, `"myapi/graph/generated"`) {
		t.Error("expected moduleName threaded into emitted import")
	}
}

func TestGenerateResolverHelpers_DefaultsModuleName(t *testing.T) {
	got := GenerateResolverHelpers(nil, "")
	if !strings.Contains(got, `"core/graph/generated"`) {
		t.Errorf("expected default 'core' moduleName, got import-less output:\n%s", got[:200])
	}
}

// --- collectServiceImports works with both inputs ---

func TestCollectServiceImports_OnlyMutations(t *testing.T) {
	got := collectServiceImports([]StaticMutationDef{{ImportPath: "a/b"}}, nil)
	if len(got) != 1 || got[0] != "a/b" {
		t.Errorf("expected [a/b], got %v", got)
	}
}

func TestCollectServiceImports_OnlyQueries(t *testing.T) {
	got := collectServiceImports(nil, []StaticQueryDef{{ImportPath: "c/d"}})
	if len(got) != 1 || got[0] != "c/d" {
		t.Errorf("expected [c/d], got %v", got)
	}
}

func TestCollectServiceImports_Empty(t *testing.T) {
	got := collectServiceImports(nil, nil)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}
