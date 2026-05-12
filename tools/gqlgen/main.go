package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Usage:
//
//	gqlgen <models-dir> <api-dir>
//
// Runs the full GraphQL codegen pipeline for a consumer API:
//   1. Scans <models-dir> for Go structs that embed mixins.GraphQLMixin.
//   2. Writes .graphql schema files to <api-dir>/graph/schema/generated/.
//   3. Writes gqlgen model bindings to <api-dir>/graph/model/bindings.go.
//   4. Writes generated resolver implementations to <api-dir>/graph/resolver/.
//   5. Runs github.com/99designs/gqlgen against <api-dir>/gqlgen.yml to emit
//      the Go GraphQL execution code (generated.go, models_gen.go).
//   6. Cleans up the intermediate gqlgen.clean.yml and _gqlgen_stubs.go.
//
// The consumer's gqlgen.yml must declare an `autobind:` entry whose first
// element is the Go import path of their models package.
func main() {
	modelsDir := "packages/backend/models"
	apiDir := "apis/v1/core"

	if len(os.Args) > 1 {
		modelsDir = os.Args[1]
	}
	if len(os.Args) > 2 {
		apiDir = os.Args[2]
	}

	if err := runPipeline(modelsDir, apiDir, true); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runPipeline is the testable extraction of main(). When run99designs is
// true (production path), it shells out to `go run github.com/99designs/gqlgen
// generate` after writing schema files. Tests pass false to exercise the
// internal pipeline without the external dependency.
func runPipeline(modelsDir, apiDir string, run99designs bool) error {
	outputDir := filepath.Join(apiDir, "graph", "schema", "generated")
	moduleName := readModuleName(apiDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	result, err := ParseModels(modelsDir)
	if err != nil {
		return fmt.Errorf("parsing models: %w", err)
	}

	services := readServicesFromConfig(apiDir)
	for _, svc := range services {
		queries, mutations, serviceTypes, err := ParseService(svc.Path, svc.Import)
		if err != nil {
			return fmt.Errorf("parsing service %s: %w", svc.Path, err)
		}
		result.StaticQueries = append(result.StaticQueries, queries...)
		result.StaticMutations = append(result.StaticMutations, mutations...)
		result.ServiceTypes = append(result.ServiceTypes, serviceTypes...)
		if len(queries)+len(mutations) > 0 {
			fmt.Printf("  service %s: %d queries, %d mutations, %d types\n", filepath.Base(svc.Path), len(queries), len(mutations), len(serviceTypes))
		}
	}

	fmt.Printf("Found %d GraphQL models, %d static mutations, %d static queries\n", len(result.Models), len(result.StaticMutations), len(result.StaticQueries))

	files := GenerateSchema(result.Models)
	files["static_mutations.graphql"] = GenerateStaticMutationsSchema(result.StaticMutations)
	files["static_queries.graphql"] = GenerateStaticQueriesSchema(result.StaticQueries)
	if len(result.ServiceTypes) > 0 {
		files["service_types.graphql"] = GenerateServiceTypesSchema(result.ServiceTypes)
	}

	for name, content := range files {
		path := filepath.Join(outputDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Printf("  wrote %s\n", path)
	}

	modelsImportPath := readAutobindFromConfig(apiDir)
	if modelsImportPath == "" {
		return fmt.Errorf("gqlgen.yml must declare an autobind entry for the consumer's models package")
	}

	bindings := GenerateBindings(result.Models, modelsImportPath)
	bindingsPath := filepath.Join(outputDir, "..", "..", "model", "bindings.go")
	if err := os.MkdirAll(filepath.Dir(bindingsPath), 0755); err != nil {
		return fmt.Errorf("creating bindings dir: %w", err)
	}
	if err := os.WriteFile(bindingsPath, []byte(bindings), 0644); err != nil {
		return fmt.Errorf("writing bindings: %w", err)
	}
	fmt.Printf("  wrote %s\n", bindingsPath)

	resolverDir := filepath.Join(outputDir, "..", "..", "resolver")
	if err := os.MkdirAll(resolverDir, 0755); err != nil {
		return fmt.Errorf("creating resolver dir: %w", err)
	}

	adminMode := strings.Contains(filepath.ToSlash(apiDir), "/admin")
	resolvers := GenerateResolvers(result.Models, result.StaticMutations, result.StaticQueries, result.ServiceTypes, moduleName, modelsImportPath, adminMode)
	resolversPath := filepath.Join(resolverDir, "crud.resolvers.go")
	if err := os.WriteFile(resolversPath, []byte(resolvers), 0644); err != nil {
		return fmt.Errorf("writing resolvers: %w", err)
	}
	fmt.Printf("  wrote %s\n", resolversPath)

	helpers := GenerateResolverHelpers(result.Models, moduleName)
	helpersPath := filepath.Join(resolverDir, "helpers.go")
	if err := os.WriteFile(helpersPath, []byte(helpers), 0644); err != nil {
		return fmt.Errorf("writing helpers: %w", err)
	}
	fmt.Printf("  wrote %s\n", helpersPath)

	writeCleanGqlgenConfig(apiDir)

	if run99designs {
		fmt.Println("Running github.com/99designs/gqlgen generate …")
		cmd := exec.Command("go", "run", "github.com/99designs/gqlgen", "generate", "--config", "gqlgen.clean.yml")
		cmd.Dir = apiDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("gqlgen generate failed: %w", err)
		}
		os.Remove(filepath.Join(apiDir, "gqlgen.clean.yml"))
		os.Remove(filepath.Join(apiDir, "graph", "resolver", "_gqlgen_stubs.go"))
	}

	fmt.Println("Schema generation complete")
	return nil
}

// writeCleanGqlgenConfig reads gqlgen.yml, strips the 'services' section, and
// writes it as gqlgen.clean.yml for the official gqlgen generate tool.
func writeCleanGqlgenConfig(apiDir string) {
	src := filepath.Join(apiDir, "gqlgen.yml")
	data, err := os.ReadFile(src)
	if err != nil {
		return
	}

	var clean strings.Builder
	inServices := false

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Detect top-level keys (no leading whitespace, not a list item)
		if len(line) > 0 && line[0] != ' ' && line[0] != '-' {
			inServices = trimmed == "services:"
		}

		if !inServices {
			clean.WriteString(line)
			clean.WriteByte('\n')
		}
	}

	dst := filepath.Join(apiDir, "gqlgen.clean.yml")
	os.WriteFile(dst, []byte(clean.String()), 0644)
}

// readModuleName reads the module name from go.mod in the given directory.
func readModuleName(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "core" // fallback
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimPrefix(line, "module ")
		}
	}
	return "core"
}

// serviceEntry represents a service defined in gqlgen.yml.
type serviceEntry struct {
	Path   string
	Import string
}

// readAutobindFromConfig returns the first autobind entry from gqlgen.yml,
// which is treated as the consumer's GORM models package import path. This
// is the path emitted into generated resolver and bindings files.
//
// Expected format:
//
//	autobind:
//	  - <consumer>/backend/models
//	  - <consumer>/backend/services/whatever
//
// Returns "" if the file is missing or autobind is empty.
func readAutobindFromConfig(apiDir string) string {
	f, err := os.Open(filepath.Join(apiDir, "gqlgen.yml"))
	if err != nil {
		return ""
	}
	defer f.Close()

	inAutobind := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if len(line) > 0 && line[0] != ' ' && line[0] != '-' {
			inAutobind = trimmed == "autobind:"
			continue
		}

		if !inAutobind {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		}
	}

	return ""
}

// readServicesFromConfig reads the services section from gqlgen.yml.
// Uses simple line-by-line parsing to avoid a YAML dependency.
//
// Expected format:
//
//	services:
//	  - path: packages/backend/services/tenant
//	    import: github.com/mashbot-co/gocore/services/tenant
func readServicesFromConfig(apiDir string) []serviceEntry {
	f, err := os.Open(filepath.Join(apiDir, "gqlgen.yml"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var services []serviceEntry
	inServices := false
	var current serviceEntry

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Detect section boundaries
		if len(line) > 0 && line[0] != ' ' && line[0] != '-' {
			if inServices && current.Path != "" {
				services = append(services, current)
				current = serviceEntry{}
			}
			inServices = trimmed == "services:"
			continue
		}

		if !inServices {
			continue
		}

		// New list item
		if strings.HasPrefix(trimmed, "- ") {
			if current.Path != "" {
				services = append(services, current)
				current = serviceEntry{}
			}
			// Handle "- path: value" on the same line
			rest := strings.TrimPrefix(trimmed, "- ")
			if strings.HasPrefix(rest, "path:") {
				current.Path = strings.TrimSpace(strings.TrimPrefix(rest, "path:"))
			}
			continue
		}

		// Key-value within a list item
		if strings.HasPrefix(trimmed, "path:") {
			current.Path = strings.TrimSpace(strings.TrimPrefix(trimmed, "path:"))
		} else if strings.HasPrefix(trimmed, "import:") {
			current.Import = strings.TrimSpace(strings.TrimPrefix(trimmed, "import:"))
		}
	}

	// Don't forget the last entry
	if inServices && current.Path != "" {
		services = append(services, current)
	}

	return services
}
