package gocore

import "testing"

func TestInit_AppliesDefaultsWhenFieldsEmpty(t *testing.T) {
	defer Reset()
	Init(Config{})
	if got := Name(); got != "gocore" {
		t.Errorf("Name default: got %q", got)
	}
	if got := StubHeaderPrefix(); got != "X-Stub-" {
		t.Errorf("StubHeaderPrefix default: got %q", got)
	}
	if got := JWTIssuer(); got != "gocore" {
		t.Errorf("JWTIssuer default: got %q", got)
	}
}

func TestInit_DerivesJWTIssuerFromName(t *testing.T) {
	defer Reset()
	Init(Config{Name: "iro-studio"})
	if got := JWTIssuer(); got != "iro-studio" {
		t.Errorf("JWTIssuer should default to Name: got %q", got)
	}
}

func TestInit_OverridesAllThree(t *testing.T) {
	defer Reset()
	Init(Config{
		Name:             "iro-studio",
		StubHeaderPrefix: "X-Iro-Stub-",
		JWTIssuer:        "iro.studio",
	})
	if got := Name(); got != "iro-studio" {
		t.Errorf("Name: got %q", got)
	}
	if got := StubHeaderPrefix(); got != "X-Iro-Stub-" {
		t.Errorf("StubHeaderPrefix: got %q", got)
	}
	if got := JWTIssuer(); got != "iro.studio" {
		t.Errorf("JWTIssuer: got %q", got)
	}
}

func TestReset_RestoresDefaults(t *testing.T) {
	Init(Config{Name: "scoped", StubHeaderPrefix: "X-Scoped-"})
	Reset()
	if got := Name(); got != "gocore" {
		t.Errorf("after Reset Name: got %q", got)
	}
	if got := StubHeaderPrefix(); got != "X-Stub-" {
		t.Errorf("after Reset prefix: got %q", got)
	}
}

func TestUninitialized_ReturnsDefaults(t *testing.T) {
	Reset()
	if got := Name(); got != "gocore" {
		t.Errorf("uninit Name: got %q", got)
	}
	if got := StubHeaderPrefix(); got != "X-Stub-" {
		t.Errorf("uninit prefix: got %q", got)
	}
	if got := JWTIssuer(); got != "gocore" {
		t.Errorf("uninit issuer: got %q", got)
	}
}
