package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readModuleName ──────────────────────────────────────────────────────────

func TestReadModuleName_ReadsFromGoMod(t *testing.T) {
	if got := readModuleName("testdata/sample_api"); got != "testapi" {
		t.Fatalf("expected 'testapi', got %q", got)
	}
}

func TestReadModuleName_FallbackForMissingGoMod(t *testing.T) {
	if got := readModuleName("testdata/nonexistent_dir"); got != "core" {
		t.Fatalf("expected fallback 'core', got %q", got)
	}
}

// readAutobindFromConfig ──────────────────────────────────────────────────

func TestReadAutobindFromConfig_ReturnsFirstEntry(t *testing.T) {
	got := readAutobindFromConfig("testdata/sample_api")
	if got != "example.com/sample/models" {
		t.Fatalf("expected first autobind entry, got %q", got)
	}
}

func TestReadAutobindFromConfig_MissingFile(t *testing.T) {
	if got := readAutobindFromConfig("testdata/nonexistent_dir"); got != "" {
		t.Fatalf("expected empty string for missing config, got %q", got)
	}
}

func TestReadAutobindFromConfig_NoAutobindSection(t *testing.T) {
	// Use a temp dir with a gqlgen.yml that has no autobind section.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gqlgen.yml"), []byte(`
schema:
  - graph/schema/*.graphql
models:
  UUID: foo
`), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if got := readAutobindFromConfig(dir); got != "" {
		t.Fatalf("expected empty string when no autobind, got %q", got)
	}
}

// readServicesFromConfig ──────────────────────────────────────────────────

func TestReadServicesFromConfig_ParsesAllEntries(t *testing.T) {
	got := readServicesFromConfig("testdata/sample_api")
	if len(got) != 2 {
		t.Fatalf("expected 2 service entries, got %d: %+v", len(got), got)
	}
	if got[0].Path != "packages/backend/services/foo" || got[0].Import != "example.com/sample/services/foo" {
		t.Errorf("entry[0]: got %+v", got[0])
	}
	if got[1].Path != "packages/backend/services/bar" || got[1].Import != "example.com/sample/services/bar" {
		t.Errorf("entry[1]: got %+v", got[1])
	}
}

func TestReadServicesFromConfig_MissingFileReturnsNil(t *testing.T) {
	got := readServicesFromConfig("testdata/nonexistent_dir")
	if got != nil {
		t.Fatalf("expected nil for missing file, got %+v", got)
	}
}

func TestReadServicesFromConfig_NoServicesSection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gqlgen.yml"), []byte(`
schema:
  - graph/schema/*.graphql
autobind:
  - x/y
`), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got := readServicesFromConfig(dir)
	if len(got) != 0 {
		t.Fatalf("expected no services, got %+v", got)
	}
}

// writeCleanGqlgenConfig ─────────────────────────────────────────────────

func TestWriteCleanGqlgenConfig_StripsServicesSection(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gqlgen.yml")
	original := `schema:
  - graph/schema/*.graphql

autobind:
  - x/y

services:
  - path: pkg/foo
    import: x/y/foo

models:
  UUID: m
`
	if err := os.WriteFile(src, []byte(original), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	writeCleanGqlgenConfig(dir)

	data, err := os.ReadFile(filepath.Join(dir, "gqlgen.clean.yml"))
	if err != nil {
		t.Fatalf("read clean: %v", err)
	}
	cleaned := string(data)
	if strings.Contains(cleaned, "services:") {
		t.Fatalf("expected services section stripped, got:\n%s", cleaned)
	}
	if !strings.Contains(cleaned, "autobind:") {
		t.Fatalf("expected autobind preserved, got:\n%s", cleaned)
	}
	if !strings.Contains(cleaned, "models:") {
		t.Fatalf("expected models preserved (last section), got:\n%s", cleaned)
	}
}

func TestWriteCleanGqlgenConfig_MissingFileIsNoOp(t *testing.T) {
	dir := t.TempDir()
	// No gqlgen.yml in dir. Should silently return without panic.
	writeCleanGqlgenConfig(dir)

	if _, err := os.Stat(filepath.Join(dir, "gqlgen.clean.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected no clean file when source missing, got err=%v", err)
	}
}
