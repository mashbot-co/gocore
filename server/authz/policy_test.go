package authz

import (
	"os"
	"path/filepath"
	"testing"
)

// stubPolicy is a Catalog substitute for tests that don't want to depend on
// any concrete consumer's scopes.yml.
type stubPolicy struct {
	allow map[string]map[string]bool // role → scope → allowed
}

func (s *stubPolicy) MatchesAll(role string, scopes []string) bool {
	for _, sc := range scopes {
		if !s.allow[role][sc] {
			return false
		}
	}
	return true
}

func TestLoadEndpointPolicy_ParsesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.yml")
	yaml := `
fields:
  me:               { scopes: [] }
  updateProject:    { scopes: [project:write] }
  deleteProject:    { scopes: [project:delete] }
public:
  - alive
  - health
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, err := LoadEndpointPolicy(path)
	if err != nil {
		t.Fatalf("LoadEndpointPolicy: %v", err)
	}
	if got, _ := p.RequiredScopes("updateProject"); len(got) != 1 || got[0] != "project:write" {
		t.Errorf("updateProject scopes: got %v", got)
	}
	if !p.IsPublic("alive") {
		t.Error("alive should be public")
	}
	if p.IsPublic("updateProject") {
		t.Error("updateProject should NOT be public")
	}
}

func TestRequiredScopes_UnknownField(t *testing.T) {
	p := &EndpointPolicy{Fields: map[string]FieldRule{}, publicSet: map[string]struct{}{}}
	if _, ok := p.RequiredScopes("missing"); ok {
		t.Error("RequiredScopes should return false for unlisted field")
	}
}
