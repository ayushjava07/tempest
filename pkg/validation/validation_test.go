package validation

import (
	"testing"
)

func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"hello", false},
		{"hello-world", false},
		{"a", false},
		{"", true},
		{"-bad", true},
		{"bad-", false},
		{"has spaces", true},
		{"UPPER", true},
		{"under_score", true},
	}
	for _, tc := range cases {
		err := ValidateName(tc.name)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateName(%q) err=%v, wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

func TestValidateNamespace(t *testing.T) {
	if err := ValidateNamespace("ok"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateNamespace(""); err == nil {
		t.Error("expected error for empty namespace")
	}
}
