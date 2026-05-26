package gocore

import "testing"

// TestInit_ScopeDefaults covers Init's ScopeName branch (deriving ScopeIDClaim
// and ScopeIDArg) and the three scope getters.
func TestInit_ScopeDefaults(t *testing.T) {
	t.Cleanup(Reset)
	Init(Config{Name: "iro", ScopeName: "project"})

	if got := ScopeName(); got != "project" {
		t.Errorf("ScopeName = %q, want project", got)
	}
	if got := ScopeIDClaim(); got != "project_id" {
		t.Errorf("ScopeIDClaim = %q, want project_id", got)
	}
	if got := ScopeIDArg(); got != "projectId" {
		t.Errorf("ScopeIDArg = %q, want projectId", got)
	}
}

// TestInit_ScopeExplicit covers the branch where the consumer overrides the
// derived scope claim/arg names.
func TestInit_ScopeExplicit(t *testing.T) {
	t.Cleanup(Reset)
	Init(Config{ScopeName: "tenant", ScopeIDClaim: "t_id", ScopeIDArg: "tArg"})

	if got := ScopeIDClaim(); got != "t_id" {
		t.Errorf("ScopeIDClaim = %q, want t_id (explicit)", got)
	}
	if got := ScopeIDArg(); got != "tArg" {
		t.Errorf("ScopeIDArg = %q, want tArg (explicit)", got)
	}
}
