package mailer

import (
	"strings"
	"testing"
)

// The bundled templates (invitation.html, access_request.html) act as both
// fixtures and runtime-shipped assets. These tests verify the templates
// render without error and that the rendering helper handles bad input.

func TestRenderTemplate_KnownTemplateSucceeds(t *testing.T) {
	out, err := renderTemplate("invitation.html", map[string]any{
		"AppURL":    "https://iro.studio",
		"OrgName":   "Acme",
		"Inviter":   "Alice",
		"AcceptURL": "https://iro.studio/accept",
	})
	if err != nil {
		t.Fatalf("expected invitation.html to render, got error: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected non-empty rendered output")
	}
}

func TestRenderTemplate_UnknownTemplateErrors(t *testing.T) {
	_, err := renderTemplate("does_not_exist.html", nil)
	if err == nil {
		t.Fatal("expected error for unknown template name")
	}
}

func TestRenderTemplate_AccessRequest(t *testing.T) {
	// The other bundled template — exercises a second template path through
	// the same embed FS and template.Must registration.
	_, err := renderTemplate("access_request.html", map[string]any{
		"AppURL": "https://iro.studio",
		"Name":   "Alice",
		"Email":  "alice@example.com",
	})
	if err != nil {
		t.Fatalf("expected access_request.html to render, got error: %v", err)
	}
}
