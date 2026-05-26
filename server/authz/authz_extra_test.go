package authz

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestWithCurrentRole_And_CurrentRole(t *testing.T) {
	if CurrentRole(context.Background()) != "" {
		t.Error("CurrentRole (unset) should be empty")
	}
	// Empty role is a no-op (context unchanged).
	if CurrentRole(WithCurrentRole(context.Background(), "")) != "" {
		t.Error("WithCurrentRole(\"\") should not set a role")
	}
	if got := CurrentRole(WithCurrentRole(context.Background(), "owner")); got != "owner" {
		t.Errorf("CurrentRole = %q, want owner", got)
	}
}

func TestMustParseCatalog(t *testing.T) {
	c := MustParseCatalog([]byte("scopes:\n  a:\n    description: x\nroles:\n  r:\n    - a\n"))
	if c == nil {
		t.Fatal("MustParseCatalog returned nil")
	}
}

func TestMustParseCatalog_PanicsOnBadCatalog(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for a role referencing an unknown scope")
		}
	}()
	MustParseCatalog([]byte("roles:\n  r:\n    - nope\n"))
}

func TestExpand(t *testing.T) {
	c := &Catalog{
		Scopes: map[string]ScopeDef{"project:read": {}, "project:write": {}, "billing:read": {}},
		Roles:  map[string][]string{"owner": {"project:*", "billing:read"}},
	}
	got := c.Expand("owner")
	for _, want := range []string{"project:read", "project:write", "billing:read"} {
		if _, ok := got[want]; !ok {
			t.Errorf("Expand(owner) missing %q", want)
		}
	}
	if len(c.Expand("unknown")) != 0 {
		t.Error("Expand of unknown role should be empty")
	}
}

func TestLoadEndpointPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.yml")
	if err := os.WriteFile(path, []byte("public:\n  - foo\nfields:\n  bar:\n    scopes: [s]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadEndpointPolicy(path)
	if err != nil {
		t.Fatalf("LoadEndpointPolicy: %v", err)
	}
	if !p.IsPublic("foo") {
		t.Error("expected foo to be public")
	}
	if _, err := LoadEndpointPolicy(filepath.Join(dir, "missing.yml")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseEndpointPolicy_InvalidYAML(t *testing.T) {
	if _, err := ParseEndpointPolicy([]byte("\tnot: [valid")); err == nil {
		t.Error("expected parse error for invalid YAML")
	}
}

// --- AuthorizeField ---

func afCatalog() *Catalog {
	return &Catalog{
		Scopes: map[string]ScopeDef{"project:read": {}, "project:write": {}},
		Roles:  map[string][]string{"owner": {"project:*"}, "viewer": {"project:read"}},
	}
}

func afPolicy(t *testing.T) *EndpointPolicy {
	t.Helper()
	p, err := ParseEndpointPolicy([]byte("public:\n  - publicField\nfields:\n  secretField:\n    scopes: [project:write]\n"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// rootCtx builds a context carrying a root-level gqlgen FieldContext for `field`.
func rootCtx(field string) context.Context {
	fc := &graphql.FieldContext{Field: graphql.CollectedField{Field: &ast.Field{Name: field, Alias: field}}}
	return graphql.WithFieldContext(context.Background(), fc)
}

func TestAuthorizeField(t *testing.T) {
	cat := afCatalog()
	ep := afPolicy(t)
	next := func(context.Context) (interface{}, error) { return "ok", nil }

	t.Run("no field context passes through", func(t *testing.T) {
		mw := AuthorizeField(cat, ep, nil)
		if _, err := mw(context.Background(), next); err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	t.Run("nested (non-root) field passes through", func(t *testing.T) {
		parent := graphql.WithFieldContext(context.Background(), &graphql.FieldContext{Field: graphql.CollectedField{Field: &ast.Field{Name: "p", Alias: "p"}}})
		child := graphql.WithFieldContext(parent, &graphql.FieldContext{Field: graphql.CollectedField{Field: &ast.Field{Name: "c", Alias: "c"}}})
		mw := AuthorizeField(cat, ep, func(context.Context, map[string]any) (string, error) { return "viewer", nil })
		if _, err := mw(child, next); err != nil {
			t.Fatalf("nested field should pass through, got %v", err)
		}
	})

	t.Run("introspection field allowed", func(t *testing.T) {
		mw := AuthorizeField(cat, ep, nil)
		if _, err := mw(rootCtx("__typename"), next); err != nil {
			t.Fatalf("introspection should pass, got %v", err)
		}
	})

	t.Run("public field allowed", func(t *testing.T) {
		mw := AuthorizeField(cat, ep, nil)
		if _, err := mw(rootCtx("publicField"), next); err != nil {
			t.Fatalf("public field should pass, got %v", err)
		}
	})

	t.Run("missing policy entry errors", func(t *testing.T) {
		mw := AuthorizeField(cat, ep, nil)
		if _, err := mw(rootCtx("unknownField"), next); err == nil {
			t.Fatal("expected config error for field with no policy entry")
		}
	})

	t.Run("resolver error propagates", func(t *testing.T) {
		mw := AuthorizeField(cat, ep, func(context.Context, map[string]any) (string, error) {
			return "", errors.New("resolve boom")
		})
		if _, err := mw(rootCtx("secretField"), next); err == nil {
			t.Fatal("expected resolver error to propagate")
		}
	})

	t.Run("insufficient role rejected", func(t *testing.T) {
		mw := AuthorizeField(cat, ep, func(context.Context, map[string]any) (string, error) { return "viewer", nil })
		if _, err := mw(rootCtx("secretField"), next); err == nil {
			t.Fatal("viewer lacks project:write — expected rejection")
		}
	})

	t.Run("authorized role passes and stashes role", func(t *testing.T) {
		var sawRole string
		check := func(ctx context.Context) (interface{}, error) {
			sawRole = CurrentRole(ctx)
			return "ok", nil
		}
		mw := AuthorizeField(cat, ep, func(context.Context, map[string]any) (string, error) { return "owner", nil })
		if _, err := mw(rootCtx("secretField"), check); err != nil {
			t.Fatalf("owner should be authorized, got %v", err)
		}
		if sawRole != "owner" {
			t.Errorf("expected role stashed in ctx, got %q", sawRole)
		}
	})
}
