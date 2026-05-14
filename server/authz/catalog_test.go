package authz

import "testing"

const testCatalog = `
scopes:
  project:read:   { description: "read project" }
  project:write:  { description: "write project" }
  project:delete: { description: "delete project" }
  member:read:    { description: "read members" }

roles:
  viewer:  [project:read, member:read]
  editor:  [project:read, project:write, member:read]
  owner:   [project:*, member:*]
`

func TestParseCatalog_HappyPath(t *testing.T) {
	c, err := ParseCatalog([]byte(testCatalog))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}
	if len(c.Scopes) != 4 {
		t.Errorf("expected 4 scopes, got %d", len(c.Scopes))
	}
	if len(c.Roles) != 3 {
		t.Errorf("expected 3 roles, got %d", len(c.Roles))
	}
}

func TestGrants_ExactAndWildcard(t *testing.T) {
	c, _ := ParseCatalog([]byte(testCatalog))
	if !c.Grants("editor", "project:write") {
		t.Errorf("editor should grant project:write")
	}
	if c.Grants("viewer", "project:write") {
		t.Errorf("viewer should NOT grant project:write")
	}
	if !c.Grants("owner", "project:delete") {
		t.Errorf("owner should grant project:delete via project:*")
	}
	if c.Grants("phantom", "project:read") {
		t.Errorf("unknown role should grant nothing")
	}
}

func TestMatchesAll(t *testing.T) {
	c, _ := ParseCatalog([]byte(testCatalog))
	if !c.MatchesAll("viewer", []string{}) {
		t.Errorf("empty scopes always satisfied")
	}
	if !c.MatchesAll("editor", []string{"project:read", "project:write"}) {
		t.Errorf("editor should satisfy read+write")
	}
	if c.MatchesAll("editor", []string{"project:read", "project:delete"}) {
		t.Errorf("editor should NOT satisfy delete")
	}
}

func TestExpand_OwnerCoversEveryProjectAndMember(t *testing.T) {
	c, _ := ParseCatalog([]byte(testCatalog))
	expanded := c.Expand("owner")
	for _, s := range []string{"project:read", "project:write", "project:delete", "member:read"} {
		if _, ok := expanded[s]; !ok {
			t.Errorf("Expand(owner) missing %q", s)
		}
	}
}

func TestValidate_RejectsUnknownScopeInRole(t *testing.T) {
	bad := `
scopes:
  project:read: { description: "" }
roles:
  phantom: [project:read, made:up]
`
	if _, err := ParseCatalog([]byte(bad)); err == nil {
		t.Error("expected parse to reject role with unknown scope")
	}
}

func TestMatchScope_PrefixGlob(t *testing.T) {
	cases := []struct {
		pattern, scope string
		want           bool
	}{
		{"project:read", "project:read", true},
		{"project:read", "project:write", false},
		{"project:*", "project:read", true},
		{"project:*", "project:read:detail", true},
		{"project:*", "member:read", false},
		{"project:*", "project", false},
	}
	for _, tc := range cases {
		if got := matchScope(tc.pattern, tc.scope); got != tc.want {
			t.Errorf("matchScope(%q, %q) = %v, want %v", tc.pattern, tc.scope, got, tc.want)
		}
	}
}
