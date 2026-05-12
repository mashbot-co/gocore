package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stagedAPIDir builds an apiDir under t.TempDir() with the minimum files the
// pipeline needs: go.mod (so readModuleName works), gqlgen.yml (so
// readAutobindFromConfig works). Returns the temp apiDir path.
func stagedAPIDir(t *testing.T, moduleName, autobind string, services string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+moduleName+"\n\ngo 1.26.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	yml := "schema:\n  - graph/schema/generated/*.graphql\n\nautobind:\n  - " + autobind + "\n"
	if services != "" {
		yml += "\n" + services
	}
	if err := os.WriteFile(filepath.Join(dir, "gqlgen.yml"), []byte(yml), 0644); err != nil {
		t.Fatalf("write gqlgen.yml: %v", err)
	}
	return dir
}

// --- runPipeline happy path against testdata/sample_models ---

func TestRunPipeline_ProducesAllExpectedFiles(t *testing.T) {
	apiDir := stagedAPIDir(t, "samplecore", "example.com/sample/models", "")

	if err := runPipeline("testdata/sample_models", apiDir, false); err != nil {
		t.Fatalf("runPipeline: %v", err)
	}

	expected := []string{
		"graph/schema/generated/types.graphql",
		"graph/schema/generated/inputs.graphql",
		"graph/schema/generated/queries.graphql",
		"graph/schema/generated/mutations.graphql",
		"graph/schema/generated/custom_mutations.graphql",
		"graph/schema/generated/connections.graphql",
		"graph/schema/generated/static_mutations.graphql",
		"graph/schema/generated/static_queries.graphql",
		"graph/model/bindings.go",
		"graph/resolver/crud.resolvers.go",
		"graph/resolver/helpers.go",
		"gqlgen.clean.yml",
	}
	for _, rel := range expected {
		if _, err := os.Stat(filepath.Join(apiDir, rel)); err != nil {
			t.Errorf("expected %s to exist after pipeline run: %v", rel, err)
		}
	}
}

func TestRunPipeline_CustomMutationsForInstanceMethods(t *testing.T) {
	apiDir := stagedAPIDir(t, "samplecore", "example.com/sample/models", "")
	if err := runPipeline("testdata/sample_models", apiDir, false); err != nil {
		t.Fatalf("runPipeline: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(apiDir, "graph/schema/generated/custom_mutations.graphql"))
	if err != nil {
		t.Fatalf("read custom_mutations.graphql: %v", err)
	}
	got := string(data)

	// AgentVersion has Promote() and Archive() methods annotated with
	// graphql:mutation. Both should emit entries in custom_mutations.graphql.
	for _, snippet := range []string{
		"promoteAgentVersion",
		"archiveAgentVersion",
		"extend type Mutation",
	} {
		if !strings.Contains(got, snippet) {
			t.Errorf("expected %q in custom_mutations.graphql:\n%s", snippet, got)
		}
	}
}

func TestRunPipeline_ServicesFromGqlgenYaml(t *testing.T) {
	services := "services:\n  - path: testdata/sample_service\n    import: example.com/sample/services/tenant\n"
	apiDir := stagedAPIDir(t, "samplecore", "example.com/sample/models", services)

	if err := runPipeline("testdata/sample_models", apiDir, false); err != nil {
		t.Fatalf("runPipeline: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(apiDir, "graph/schema/generated/static_mutations.graphql"))
	got := string(data)
	if !strings.Contains(got, "switchTenant") {
		t.Errorf("expected switchTenant in static_mutations.graphql from service:\n%s", got)
	}
}

// --- runPipeline error paths ---

func TestRunPipeline_NonexistentModelsDirReturnsError(t *testing.T) {
	apiDir := stagedAPIDir(t, "samplecore", "example.com/sample/models", "")
	err := runPipeline("testdata/this_does_not_exist", apiDir, false)
	if err == nil {
		t.Fatal("expected error for missing models dir")
	}
	if !strings.Contains(err.Error(), "parsing models") {
		t.Errorf("expected wrapped 'parsing models' error, got: %v", err)
	}
}

func TestRunPipeline_MissingAutobindReturnsError(t *testing.T) {
	// Stage an apiDir with NO autobind in gqlgen.yml.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.26.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gqlgen.yml"), []byte("schema:\n  - g.graphql\n"), 0644); err != nil {
		t.Fatalf("write gqlgen.yml: %v", err)
	}

	err := runPipeline("testdata/sample_models", dir, false)
	if err == nil {
		t.Fatal("expected error when autobind is missing")
	}
	if !strings.Contains(err.Error(), "autobind") {
		t.Errorf("expected autobind error message, got: %v", err)
	}
}

func TestRunPipeline_NonexistentServiceDirReturnsError(t *testing.T) {
	services := "services:\n  - path: testdata/missing_service\n    import: example.com/x\n"
	apiDir := stagedAPIDir(t, "samplecore", "example.com/sample/models", services)
	err := runPipeline("testdata/sample_models", apiDir, false)
	if err == nil {
		t.Fatal("expected error for missing service dir")
	}
	if !strings.Contains(err.Error(), "service") {
		t.Errorf("expected wrapped service error, got: %v", err)
	}
}

// --- Force runPipeline I/O error paths to surface ---

// When the output directory path collides with an existing file (not a
// directory), os.MkdirAll returns an error and runPipeline should propagate it.
func TestRunPipeline_MkdirAllFailureSurfacesError(t *testing.T) {
	dir := t.TempDir()

	// Make `dir/graph` a regular file so MkdirAll fails when it tries to
	// create the schema-generated subdirectory.
	if err := os.WriteFile(filepath.Join(dir, "graph"), []byte("blocker"), 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.26.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gqlgen.yml"), []byte("autobind:\n  - x/y\n"), 0644); err != nil {
		t.Fatalf("write gqlgen.yml: %v", err)
	}

	err := runPipeline("testdata/sample_models", dir, false)
	if err == nil {
		t.Fatal("expected error when output dir path is blocked by a file")
	}
	if !strings.Contains(err.Error(), "creating") {
		t.Errorf("expected wrapped 'creating' error, got: %v", err)
	}
}

// main() is just args parsing + os.Exit(runPipeline(...)). The body is hard
// to test directly because of the os.Exit call, but we can exercise the
// happy path by setting os.Args and invoking main() in a subprocess via
// the standard Go testing pattern.
func TestMain_HappyPathInSubprocess(t *testing.T) {
	if os.Getenv("GQLGEN_MAIN_SUBPROCESS") == "1" {
		// Subprocess: actually run main() with prepared args. The CWD will be
		// the package directory (where the test binary runs from).
		os.Args = []string{"gqlgen", "testdata/sample_models", t.TempDir()}
		// Stage gqlgen.yml in the temp apiDir.
		apiDir := os.Args[2]
		_ = os.WriteFile(filepath.Join(apiDir, "go.mod"), []byte("module x\n\ngo 1.26.0\n"), 0644)
		_ = os.WriteFile(filepath.Join(apiDir, "gqlgen.yml"), []byte("autobind:\n  - example.com/x\n"), 0644)
		main()
		return
	}
	// Driver: re-exec this test in a subprocess to actually run main().
	// main() always tries to invoke `go run github.com/99designs/gqlgen` at
	// the end — which fails in this test environment because the staged
	// apiDir has no Go source the gqlgen library can compile. That's fine;
	// what we want to verify is that main() runs far enough through arg
	// parsing and runPipeline to exercise its lines. The 99designs step
	// failing causes os.Exit(1) in the child, which we ignore.
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_HappyPathInSubprocess", "-test.v")
	cmd.Env = append(os.Environ(), "GQLGEN_MAIN_SUBPROCESS=1")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "Found ") {
		t.Fatalf("subprocess main() didn't reach runPipeline's status print:\n%s", out)
	}
}

// Tests for ParseModels' error-handling path when one of its files contains
// invalid Go syntax — should fail at parser.ParseFile.
func TestParseModels_RejectsInvalidGoSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package this is not valid go"), 0644); err != nil {
		t.Fatalf("write broken: %v", err)
	}
	_, err := ParseModels(dir)
	if err == nil {
		t.Fatal("expected error from invalid Go source")
	}
}

// silence unused-import warning
var _ = errors.New
