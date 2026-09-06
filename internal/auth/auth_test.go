package auth

import (
	"net/http"
	"testing"
)

func TestRole_Allow(t *testing.T) {
	cases := []struct {
		role   Role
		action Action
		want   bool
	}{
		{RoleReader, ActionRead, true},
		{RoleReader, ActionSubmit, false},
		{RoleOperator, ActionRead, true},
		{RoleOperator, ActionSubmit, true},
		{RoleOperator, ActionDelete, false},
		{RoleAdmin, ActionDelete, true},
		{RoleAdmin, ActionManageTokens, true},
		{"unknown", ActionRead, false},
	}
	for _, tc := range cases {
		if got := tc.role.Allow(tc.action); got != tc.want {
			t.Errorf("Role(%s).Allow(%s) = %v, want %v", tc.role, tc.action, got, tc.want)
		}
	}
}

func TestParseRole(t *testing.T) {
	r, err := ParseRole("admin")
	if err != nil || r != RoleAdmin {
		t.Errorf("ParseRole(admin) = %v, %v", r, err)
	}
	_, err = ParseRole("bogus")
	if err == nil {
		t.Error("expected error for bogus role")
	}
}

func TestHashToken(t *testing.T) {
	h1 := HashToken("secret")
	h2 := HashToken("secret")
	if h1 != h2 {
		t.Error("expected same hash for same input")
	}
	if h1 == "secret" {
		t.Error("expected hashed value, not plaintext")
	}
}

func TestBearerToken(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer tok123")
	tok, ok := BearerToken(h)
	if !ok || tok != "tok123" {
		t.Errorf("expected tok123, got %q, %v", tok, ok)
	}
}

func TestBearerToken_NoAuth(t *testing.T) {
	h := http.Header{}
	_, ok := BearerToken(h)
	if ok {
		t.Error("expected false for missing Authorization")
	}
}

func TestBearerToken_NotBearer(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Basic abc")
	_, ok := BearerToken(h)
	if ok {
		t.Error("expected false for non-Bearer auth")
	}
}
