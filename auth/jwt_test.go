package auth

import "testing"

func TestInitKeys_NoOp(t *testing.T) {
	if err := InitKeys(); err != nil {
		t.Fatalf("InitKeys: expected nil, got %v", err)
	}
}

func TestValidateToken_NotImplemented(t *testing.T) {
	claims, err := ValidateToken("any.bearer.token")
	if err == nil {
		t.Fatal("expected stub error from ValidateToken")
	}
	if claims != nil {
		t.Fatalf("expected nil claims, got %+v", claims)
	}
}
